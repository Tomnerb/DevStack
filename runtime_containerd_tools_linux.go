//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type containerdCPUSample struct {
	UsageUsec uint64
	At        time.Time
}

type containerdToolState struct {
	Mu         sync.Mutex
	CPUSamples map[string]containerdCPUSample
	Terminals  map[string]*containerdToolSession
}

type containerdToolSession struct {
	Process containerd.Process
	Stdin   *io.PipeWriter
	Output  *containerdToolBuffer
}

type containerdToolBuffer struct {
	Mu     sync.Mutex
	Base   int64
	Data   []byte
	Closed bool
	Error  string
}

var containerdToolStates sync.Map

func (r *containerdRuntime) toolState() *containerdToolState {
	if value, ok := containerdToolStates.Load(r); ok {
		return value.(*containerdToolState)
	}

	state := &containerdToolState{
		CPUSamples: make(map[string]containerdCPUSample),
		Terminals:  make(map[string]*containerdToolSession),
	}

	actual, _ := containerdToolStates.LoadOrStore(r, state)
	return actual.(*containerdToolState)
}

func containerdLogPath(id string) (string, error) {
	stateDir := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))

	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateDir = filepath.Join(home, ".local", "state")
	}

	dir := filepath.Join(stateDir, "DevStack", "container-logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	return filepath.Join(dir, sanitizeContainerdID(id)+".log"), nil
}

func (r *containerdRuntime) containerTaskIO(id string) cio.Creator {
	path, err := containerdLogPath(id)
	if err != nil {
		return cio.NullIO
	}

	return cio.LogFile(path)
}

func (r *containerdRuntime) RuntimeReadLogs(
	ctx context.Context,
	id string,
	tail int,
	offset int64,
) (RuntimeLogChunk, error) {
	path, err := containerdLogPath(id)
	if err != nil {
		return RuntimeLogChunk{}, err
	}

	full, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeLogChunk{}, nil
		}
		return RuntimeLogChunk{}, err
	}

	if offset < 0 {
		offset = 0
	}

	if tail > 0 && offset == 0 {
		lines := bytes.Split(full, []byte("\n"))
		data := full

		if len(lines) > tail+1 {
			data = bytes.Join(lines[len(lines)-tail-1:], []byte("\n"))
		}

		return RuntimeLogChunk{
			Data:       string(data),
			NextOffset: int64(len(full)),
		}, nil
	}

	if offset > int64(len(full)) {
		offset = int64(len(full))
	}

	return RuntimeLogChunk{
		Data:       string(full[offset:]),
		NextOffset: int64(len(full)),
	}, nil
}

func (r *containerdRuntime) RuntimeStats(
	ctx context.Context,
	ids []string,
) []ContainerResourceStats {
	state := r.toolState()
	result := make([]ContainerResourceStats, 0, len(ids))

	for _, id := range ids {
		item := ContainerResourceStats{ID: id}

		container, err := r.client.LoadContainer(ctx, id)
		if err != nil {
			item.Error = err.Error()
			result = append(result, item)
			continue
		}

		task, err := container.Task(ctx, nil)
		if err != nil {
			item.Error = err.Error()
			result = append(result, item)
			continue
		}

		values, err := readContainerCgroupV2(task.Pid())
		if err != nil {
			item.Error = err.Error()
			result = append(result, item)
			continue
		}

		item.MemoryUsage = values.MemoryCurrent
		item.MemoryLimit = values.MemoryMax
		item.PIDs = values.PIDs

		if item.MemoryLimit > 0 {
			item.MemoryPercent = float64(item.MemoryUsage) / float64(item.MemoryLimit) * 100
		}

		now := time.Now()

		state.Mu.Lock()
		previous, found := state.CPUSamples[id]
		state.CPUSamples[id] = containerdCPUSample{
			UsageUsec: values.CPUUsec,
			At:        now,
		}
		state.Mu.Unlock()

		if found && values.CPUUsec >= previous.UsageUsec {
			wall := now.Sub(previous.At)
			if wall > 0 {
				cpuNS := (values.CPUUsec - previous.UsageUsec) * 1000
				item.CPUPercent = float64(cpuNS) / float64(wall.Nanoseconds()) * 100
			}
		}

		result = append(result, item)
	}

	return result
}

type cgroupV2Values struct {
	CPUUsec       uint64
	MemoryCurrent uint64
	MemoryMax     uint64
	PIDs          uint64
}

func readContainerCgroupV2(pid uint32) (cgroupV2Values, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return cgroupV2Values{}, err
	}

	var group string

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			group = strings.TrimPrefix(line, "0::")
			break
		}
	}

	if group == "" {
		return cgroupV2Values{}, errors.New("cgroup v2 path not found")
	}

	base := filepath.Join("/sys/fs/cgroup", filepath.Clean(group))
	values := cgroupV2Values{}

	cpuData, _ := os.ReadFile(filepath.Join(base, "cpu.stat"))
	for _, line := range strings.Split(string(cpuData), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			values.CPUUsec, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	values.MemoryCurrent = readContainerUint(filepath.Join(base, "memory.current"))

	maxData, _ := os.ReadFile(filepath.Join(base, "memory.max"))
	maxText := strings.TrimSpace(string(maxData))
	if maxText != "" && maxText != "max" {
		values.MemoryMax, _ = strconv.ParseUint(maxText, 10, 64)
	}

	values.PIDs = readContainerUint(filepath.Join(base, "pids.current"))

	return values, nil
}

func readContainerUint(path string) uint64 {
	data, _ := os.ReadFile(path)
	value, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return value
}

func (r *containerdRuntime) RuntimeOpenTerminal(
	ctx context.Context,
	containerID string,
	width uint,
	height uint,
) (string, error) {
	state := r.toolState()

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return "", err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return "", err
	}

	spec, err := container.Spec(ctx)
	if err != nil || spec.Process == nil {
		return "", errors.New("OCI process spec unavailable")
	}

	processSpec := *spec.Process
	processSpec.Terminal = true
	processSpec.Args = []string{"/bin/sh"}
	processSpec.Cwd = "/"
	processSpec.Env = append(processSpec.Env, "TERM=xterm-256color")

	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 28
	}

	processSpec.ConsoleSize = &specs.Box{
		Height: uint(height),
		Width:  uint(width),
	}

	sessionID := fmt.Sprintf("x%x", time.Now().UnixNano())
	stdinReader, stdinWriter := io.Pipe()
	output := &containerdToolBuffer{}

	creator := cio.NewCreator(
		cio.WithStreams(stdinReader, output, output),
		cio.WithTerminal,
	)

	process, err := task.Exec(ctx, sessionID, &processSpec, creator)
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		return "", err
	}

	exitC, err := process.Wait(context.Background())
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_, _ = process.Delete(ctx, containerd.WithProcessKill)
		return "", err
	}

	if err := process.Start(ctx); err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_, _ = process.Delete(ctx, containerd.WithProcessKill)
		return "", err
	}

	state.Mu.Lock()
	state.Terminals[sessionID] = &containerdToolSession{
		Process: process,
		Stdin:   stdinWriter,
		Output:  output,
	}
	state.Mu.Unlock()

	go func() {
		status := <-exitC
		_, _, runErr := status.Result()

		process.IO().Wait()
		_ = process.IO().Close()
		_ = stdinReader.Close()
		_ = stdinWriter.Close()

		output.finish(runErr)
	}()

	return sessionID, nil
}

func (r *containerdRuntime) RuntimeTerminalInput(
	ctx context.Context,
	sessionID string,
	input string,
) error {
	state := r.toolState()

	state.Mu.Lock()
	session := state.Terminals[sessionID]
	state.Mu.Unlock()

	if session == nil {
		return errors.New("terminal session closed")
	}

	_, err := session.Stdin.Write([]byte(input))
	return err
}

func (r *containerdRuntime) RuntimeTerminalResize(
	ctx context.Context,
	sessionID string,
	width uint,
	height uint,
) error {
	state := r.toolState()

	state.Mu.Lock()
	session := state.Terminals[sessionID]
	state.Mu.Unlock()

	if session == nil {
		return errors.New("terminal session closed")
	}

	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 28
	}

	return session.Process.Resize(ctx, uint32(width), uint32(height))
}

func (r *containerdRuntime) RuntimeTerminalOutput(
	ctx context.Context,
	sessionID string,
	offset int64,
) (RuntimeTerminalChunk, error) {
	state := r.toolState()

	state.Mu.Lock()
	session := state.Terminals[sessionID]
	state.Mu.Unlock()

	if session == nil {
		return RuntimeTerminalChunk{
			NextOffset: offset,
			Closed:     true,
		}, nil
	}

	data, next, closed, errText := session.Output.after(offset)

	return RuntimeTerminalChunk{
		Data:       data,
		NextOffset: next,
		Closed:     closed,
		Error:      errText,
	}, nil
}

func (r *containerdRuntime) RuntimeTerminalClose(
	ctx context.Context,
	sessionID string,
) error {
	state := r.toolState()

	state.Mu.Lock()
	session := state.Terminals[sessionID]
	delete(state.Terminals, sessionID)
	state.Mu.Unlock()

	if session == nil {
		return nil
	}

	_ = session.Stdin.Close()
	_ = session.Process.Kill(ctx, syscall.SIGKILL)
	_, _ = session.Process.Delete(ctx, containerd.WithProcessKill)
	session.Output.finish(nil)

	return nil
}

func (b *containerdToolBuffer) Write(data []byte) (int, error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	n := len(data)
	b.Data = append(b.Data, data...)

	if len(b.Data) > 2<<20 {
		drop := len(b.Data) - (2 << 20)
		b.Data = append([]byte(nil), b.Data[drop:]...)
		b.Base += int64(drop)
	}

	return n, nil
}

func (b *containerdToolBuffer) after(offset int64) (string, int64, bool, string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	if offset < b.Base {
		offset = b.Base
	}

	index := offset - b.Base
	if index < 0 {
		index = 0
	}
	if index > int64(len(b.Data)) {
		index = int64(len(b.Data))
	}

	return string(b.Data[index:]), b.Base + int64(len(b.Data)), b.Closed, b.Error
}

func (b *containerdToolBuffer) finish(err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	b.Closed = true
	if err != nil {
		b.Error = err.Error()
	}
}
