package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type trayContainerGroup struct {
	Name       string
	Project    string
	Standalone bool
	Containers []ContainerInfo
}

type TrayController struct {
	app           *application.App
	window        *application.WebviewWindow
	tray          *application.SystemTray
	dockerService *DockerService
	appService    *AppService
	icon          []byte

	actionMu  sync.Mutex
	busy      bool
	busyLabel string
	lastError string

	refreshMu    sync.Mutex
	refreshTimer *time.Timer
}

func NewTrayController(
	app *application.App,
	window *application.WebviewWindow,
	dockerService *DockerService,
	appService *AppService,
	icon []byte,
) *TrayController {
	return &TrayController{
		app:           app,
		window:        window,
		dockerService: dockerService,
		appService:    appService,
		icon:          icon,
	}
}

func (t *TrayController) Start() {
	t.tray = t.app.SystemTray.New()
	t.tray.SetIcon(t.icon)
	t.tray.SetTooltip("DevStack")

	// Do not attach a tray-level OnClick handler.
	// Clicking the status item should only open the native menu.
	t.refresh()

	// Rebuild the tray only when the runtime actually reports a change.
	// DockerService already emits this event for Docker/container lifecycle
	// changes. The callback is lightweight and the refresh itself is
	// debounced below.
	t.app.Event.On(
		"devstack:docker-event",
		func(event *application.CustomEvent) {
			t.scheduleRefresh()
		},
	)
}

func (t *TrayController) Close() {
	t.refreshMu.Lock()
	if t.refreshTimer != nil {
		t.refreshTimer.Stop()
		t.refreshTimer = nil
	}
	t.refreshMu.Unlock()

	if t.tray != nil {
		t.tray.Destroy()
	}
}

func (t *TrayController) scheduleRefresh() {
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	if t.refreshTimer != nil {
		t.refreshTimer.Stop()
	}

	t.refreshTimer = time.AfterFunc(
		250*time.Millisecond,
		func() {
			t.refresh()
		},
	)
}

func (t *TrayController) refresh() {
	if t.tray == nil {
		return
	}

	settings := t.appService.Snapshot()
	engineStatus := t.dockerService.GetEngineStatus(
		settings.EngineBackend,
		settings.WSLDistro,
	)

	runtimeOverview := t.dockerService.GetRuntimeOverview()

	containers := []ContainerInfo{}
	containerErr := error(nil)

	if runtimeOverview.Active.Connected {
		containers, containerErr =
			t.dockerService.ListContainers()
	}

	sort.SliceStable(
		containers,
		func(i, j int) bool {
			iRunning := containerIsRunning(
				containers[i],
			)
			jRunning := containerIsRunning(
				containers[j],
			)

			if iRunning != jRunning {
				return iRunning
			}

			return strings.ToLower(
				containerTrayName(
					containers[i],
				),
			) <
				strings.ToLower(
					containerTrayName(
						containers[j],
					),
				)
		},
	)

	running, stopped := countTrayContainers(
		containers,
	)

	t.tray.SetTooltip(
		trayTooltip(
			engineStatus,
			runtimeOverview.Active,
			running,
			stopped,
		),
	)

	menu := t.app.NewMenu()

	statusLabel := trayEngineStatusLabel(
		engineStatus,
		runtimeOverview.Active,
	)
	menu.Add(statusLabel).
		SetEnabled(false)

	if containerErr == nil {
		menu.Add(
			fmt.Sprintf(
				"Containers: %d running · %d stopped",
				running,
				stopped,
			),
		).SetEnabled(false)
	} else {
		menu.Add("Containers: unavailable").
			SetEnabled(false)
	}

	t.actionMu.Lock()
	busy := t.busy
	busyLabel := t.busyLabel
	lastError := t.lastError
	t.actionMu.Unlock()

	if busy {
		menu.Add("Working: " + busyLabel + "…").
			SetEnabled(false)
	}

	if lastError != "" {
		menu.Add(
			"Last error: " +
				shortTrayText(
					lastError,
					74,
				),
		).SetEnabled(false)
	}

	menu.AddSeparator()

	t.addEngineMenu(
		menu,
		settings,
		engineStatus,
		busy,
	)

	t.addContainerMenu(
		menu,
		containers,
		containerErr,
		running,
		stopped,
		busy,
	)

	menu.AddSeparator()

	if t.window.IsVisible() {
		menu.Add("Hide DevStack").
			OnClick(
				func(
					ctx *application.Context,
				) {
					t.window.Hide()
				},
			)
	} else {
		menu.Add("Show DevStack").
			OnClick(
				func(
					ctx *application.Context,
				) {
					t.window.Show()
					t.window.Focus()
				},
			)
	}

	menu.Add("Refresh").
		OnClick(
			func(
				ctx *application.Context,
			) {
				t.refresh()
				t.emitTrayRefresh()
			},
		)

	startAtLogin :=
		menu.AddCheckbox(
			"Start at Login",
			settings.StartAtLogin,
		)

	startAtLogin.OnClick(
		func(
			ctx *application.Context,
		) {
			enabled :=
				ctx.ClickedMenuItem().
					Checked()

			t.runAction(
				"Updating login setting",
				func() error {
					next :=
						t.appService.
							Snapshot()

					next.StartAtLogin =
						enabled

					return t.appService.
						UpdateSettings(
							next,
						)
				},
			)
		},
	)

	menu.AddSeparator()

	menu.Add("Quit DevStack").
		OnClick(
			func(
				ctx *application.Context,
			) {
				t.app.Quit()
			},
		)

	t.tray.SetMenu(menu)
}

func (t *TrayController) addEngineMenu(
	menu *application.Menu,
	settings AppSettings,
	status EngineStatus,
	busy bool,
) {
	engineMenu := menu.AddSubmenu("Engine")

	engineMenu.Add(
		trayEngineStatusLabel(
			status,
			t.dockerService.
				GetRuntimeOverview().
				Active,
		),
	).SetEnabled(false)

	engineMenu.AddSeparator()

	if !status.Running {
		startItem :=
			engineMenu.Add(
				"Start / Connect Engine",
			)

		startItem.SetEnabled(
			!busy,
		)

		startItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Starting engine",
					func() error {
						_, err :=
							t.dockerService.
								StartManagedEngine(
									settings.
										EngineBackend,
									settings.
										WSLDistro,
								)

						return err
					},
				)
			},
		)

		return
	}

	if t.engineCanStop(
		settings,
		status,
	) {
		stopItem :=
			engineMenu.Add(
				"Stop Engine",
			)

		stopItem.SetEnabled(
			!busy,
		)

		stopItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Stopping engine",
					func() error {
						_, err :=
							t.dockerService.
								StopManagedEngine(
									settings.
										EngineBackend,
									settings.
										WSLDistro,
								)

						return err
					},
				)
			},
		)

		restartItem :=
			engineMenu.Add(
				"Restart Engine",
			)

		restartItem.SetEnabled(
			!busy,
		)

		restartItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Restarting engine",
					func() error {
						if _, err :=
							t.dockerService.
								StopManagedEngine(
									settings.
										EngineBackend,
									settings.
										WSLDistro,
								); err != nil {
							return err
						}

						_, err :=
							t.dockerService.
								StartManagedEngine(
									settings.
										EngineBackend,
									settings.
										WSLDistro,
								)

						return err
					},
				)
			},
		)

		return
	}

	engineMenu.Add(
		"Engine is managed externally",
	).SetEnabled(false)

	reconnect :=
		engineMenu.Add(
			"Reconnect Engine",
		)

	reconnect.SetEnabled(!busy)

	reconnect.OnClick(
		func(
			ctx *application.Context,
		) {
			t.runAction(
				"Reconnecting engine",
				func() error {
					if settings.
						EngineBackend ==
						"external" ||
						trayStatusUsesExternal(
							status,
						) {
						return t.dockerService.
							ReconnectExternalDocker()
					}

					_, err :=
						t.dockerService.
							StartManagedEngine(
								settings.
									EngineBackend,
								settings.
									WSLDistro,
							)

					return err
				},
			)
		},
	)
}

func (t *TrayController) addContainerMenu(
	menu *application.Menu,
	containers []ContainerInfo,
	containerErr error,
	running int,
	stopped int,
	busy bool,
) {
	label := "Containers"

	if containerErr == nil {
		label = fmt.Sprintf(
			"Containers (%d)",
			len(containers),
		)
	}

	containerMenu := menu.AddSubmenu(label)

	if containerErr != nil {
		containerMenu.Add(
			"Runtime unavailable",
		).SetEnabled(false)
		return
	}

	if len(containers) == 0 {
		containerMenu.Add(
			"No containers",
		).SetEnabled(false)
		return
	}

	t.addAllContainersActions(
		containerMenu,
		containers,
		running,
		stopped,
		busy,
	)

	groups := groupTrayContainers(containers)

	if len(groups) > 0 {
		containerMenu.AddSeparator()
	}

	for _, group := range groups {
		t.addContainerGroupMenu(
			containerMenu,
			group,
			busy,
		)
	}

}

func (t *TrayController) addAllContainersActions(
	menu *application.Menu,
	containers []ContainerInfo,
	running int,
	stopped int,
	busy bool,
) {
	if stopped > 0 {
		startAll := menu.Add(
			fmt.Sprintf(
				"Start All Stopped (%d)",
				stopped,
			),
		)

		startAll.SetEnabled(!busy)

		startAll.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Starting all stopped containers",
					func() error {
						return t.startTrayContainers(
							containers,
						)
					},
				)
			},
		)
	}

	if running > 0 {
		stopAll := menu.Add(
			fmt.Sprintf(
				"Stop All Running (%d)",
				running,
			),
		)

		stopAll.SetEnabled(!busy)

		stopAll.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Stopping all running containers",
					func() error {
						return t.stopTrayContainers(
							containers,
						)
					},
				)
			},
		)
	}

	if running > 0 {
		restartAll := menu.Add(
			fmt.Sprintf(
				"Restart All Running (%d)",
				running,
			),
		)

		restartAll.SetEnabled(!busy)

		restartAll.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Restarting all running containers",
					func() error {
						return t.restartTrayContainers(
							containers,
						)
					},
				)
			},
		)
	}
}

func (t *TrayController) addContainerGroupMenu(
	parent *application.Menu,
	group trayContainerGroup,
	busy bool,
) {
	running, stopped := countTrayContainers(
		group.Containers,
	)

	groupLabel := fmt.Sprintf(
		"%s (%d)",
		group.Name,
		len(group.Containers),
	)

	groupMenu := parent.AddSubmenu(
		groupLabel,
	)

	statusText := fmt.Sprintf(
		"%d running · %d stopped",
		running,
		stopped,
	)

	if group.Standalone {
		statusText += " · standalone"
	} else {
		statusText += " · Compose project"
	}

	groupMenu.Add(
		statusText,
	).SetEnabled(false)

	if len(group.Containers) > 0 {
		groupMenu.AddSeparator()
	}

	if stopped > 0 {
		startGroup := groupMenu.Add(
			fmt.Sprintf(
				"Start All Stopped (%d)",
				stopped,
			),
		)

		startGroup.SetEnabled(!busy)

		startGroup.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Starting "+group.Name,
					func() error {
						return t.startTrayContainers(
							group.Containers,
						)
					},
				)
			},
		)
	}

	if running > 0 {
		stopGroup := groupMenu.Add(
			fmt.Sprintf(
				"Stop All Running (%d)",
				running,
			),
		)

		stopGroup.SetEnabled(!busy)

		stopGroup.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Stopping "+group.Name,
					func() error {
						return t.stopTrayContainers(
							group.Containers,
						)
					},
				)
			},
		)

		restartGroup := groupMenu.Add(
			fmt.Sprintf(
				"Restart Running (%d)",
				running,
			),
		)

		restartGroup.SetEnabled(!busy)

		restartGroup.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Restarting "+group.Name,
					func() error {
						return t.restartTrayContainers(
							group.Containers,
						)
					},
				)
			},
		)
	}

	if stopped > 0 || running > 0 {
		groupMenu.AddSeparator()
	}

	for _, container := range group.Containers {
		t.addSingleContainerMenu(
			groupMenu,
			container,
			busy,
		)
	}

}

func (t *TrayController) addSingleContainerMenu(
	parent *application.Menu,
	container ContainerInfo,
	busy bool,
) {
	displayName := containerTrayItemName(
		container,
	)

	statePrefix := "○"
	if containerIsRunning(container) {
		statePrefix = "●"
	}

	itemMenu := parent.AddSubmenu(
		fmt.Sprintf(
			"%s %s",
			statePrefix,
			shortTrayText(
				displayName,
				38,
			),
		),
	)

	status := strings.TrimSpace(
		container.Status,
	)

	if status == "" {
		status = container.State
	}

	itemMenu.Add(
		shortTrayText(
			status,
			56,
		),
	).SetEnabled(false)

	if len(container.Ports) > 0 {
		for _, port := range container.Ports {
			portLabel := port.Display

			if port.URL != "" {
				portLabel += " · " + port.URL
			}

			itemMenu.Add(
				shortTrayText(
					portLabel,
					60,
				),
			).SetEnabled(false)
		}
	}

	itemMenu.AddSeparator()

	if containerIsRunning(container) {
		stopItem := itemMenu.Add("Stop")
		stopItem.SetEnabled(!busy)

		stopItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Stopping "+displayName,
					func() error {
						return t.dockerService.
							StopContainer(
								container.ID,
							)
					},
				)
			},
		)

		restartItem := itemMenu.Add("Restart")
		restartItem.SetEnabled(!busy)

		restartItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Restarting "+displayName,
					func() error {
						return t.dockerService.
							RestartContainer(
								container.ID,
							)
					},
				)
			},
		)
	} else {
		startItem := itemMenu.Add("Start")
		startItem.SetEnabled(!busy)

		startItem.OnClick(
			func(
				ctx *application.Context,
			) {
				t.runAction(
					"Starting "+displayName,
					func() error {
						return t.dockerService.
							StartContainer(
								container.ID,
							)
					},
				)
			},
		)
	}
}

func (t *TrayController) startTrayContainers(
	containers []ContainerInfo,
) error {
	var failures []string

	for _, container := range containers {
		if containerIsRunning(container) {
			continue
		}

		if err := t.dockerService.
			StartContainer(
				container.ID,
			); err != nil {
			failures = append(
				failures,
				fmt.Sprintf(
					"%s: %v",
					containerTrayName(
						container,
					),
					err,
				),
			)
		}
	}

	return joinTrayFailures(failures)
}

func (t *TrayController) stopTrayContainers(
	containers []ContainerInfo,
) error {
	var failures []string

	for _, container := range containers {
		if !containerIsRunning(container) {
			continue
		}

		if err := t.dockerService.
			StopContainer(
				container.ID,
			); err != nil {
			failures = append(
				failures,
				fmt.Sprintf(
					"%s: %v",
					containerTrayName(
						container,
					),
					err,
				),
			)
		}
	}

	return joinTrayFailures(failures)
}

func (t *TrayController) restartTrayContainers(
	containers []ContainerInfo,
) error {
	var failures []string

	for _, container := range containers {
		if !containerIsRunning(container) {
			continue
		}

		if err := t.dockerService.
			RestartContainer(
				container.ID,
			); err != nil {
			failures = append(
				failures,
				fmt.Sprintf(
					"%s: %v",
					containerTrayName(
						container,
					),
					err,
				),
			)
		}
	}

	return joinTrayFailures(failures)
}

func (t *TrayController) runAction(
	label string,
	action func() error,
) {
	t.actionMu.Lock()

	if t.busy {
		t.actionMu.Unlock()
		return
	}

	t.busy = true
	t.busyLabel = label
	t.lastError = ""

	t.actionMu.Unlock()

	t.refresh()

	go func() {
		err := action()

		t.actionMu.Lock()
		t.busy = false
		t.busyLabel = ""

		if err != nil {
			t.lastError = err.Error()
		} else {
			t.lastError = ""
		}

		t.actionMu.Unlock()

		t.emitTrayRefresh()
		t.scheduleRefresh()
	}()
}

func (t *TrayController) emitTrayRefresh() {
	t.app.Event.Emit(
		"devstack:tray-refresh",
		map[string]any{
			"timestamp": time.Now().
				UnixMilli(),
		},
	)
}

func (t *TrayController) engineCanStop(
	settings AppSettings,
	status EngineStatus,
) bool {
	if !status.Running {
		return false
	}

	if settings.EngineBackend ==
		"external" ||
		trayStatusUsesExternal(
			status,
		) {
		return false
	}

	switch runtime.GOOS {
	case "darwin":
		return settings.EngineBackend ==
			"vz" ||
			settings.EngineBackend ==
				"auto"

	case "windows":
		return settings.EngineBackend ==
			"wsl2" ||
			settings.EngineBackend ==
				"auto"

	default:
		// Linux host Docker/containerd should not be stopped by
		// an unprivileged desktop utility.
		return false
	}
}

func trayStatusUsesExternal(
	status EngineStatus,
) bool {
	message :=
		strings.ToLower(
			status.Message,
		)

	return strings.Contains(
		message,
		"external",
	) ||
		strings.Contains(
			message,
			"active docker endpoint",
		)
}

func trayEngineStatusLabel(
	engine EngineStatus,
	runtimeInfo ContainerRuntimeInfo,
) string {
	state := "Stopped"
	if engine.Running {
		state = "Running"
	}

	runtimeName :=
		runtimeInfo.DisplayName

	if runtimeName == "" {
		runtimeName =
			runtimeInfo.Provider
	}

	if runtimeName == "" {
		return "Engine: " + state
	}

	return fmt.Sprintf(
		"Engine: %s · %s",
		state,
		shortTrayText(
			runtimeName,
			32,
		),
	)
}

func trayTooltip(
	engine EngineStatus,
	runtimeInfo ContainerRuntimeInfo,
	running int,
	stopped int,
) string {
	state := "Stopped"
	if engine.Running {
		state = "Running"
	}

	runtimeName :=
		runtimeInfo.Provider

	if runtimeName == "" {
		runtimeName = "runtime"
	}

	return fmt.Sprintf(
		"DevStack · Engine %s · %s · %d running · %d stopped",
		state,
		runtimeName,
		running,
		stopped,
	)
}

func groupTrayContainers(
	containers []ContainerInfo,
) []trayContainerGroup {
	projectGroups := map[string][]ContainerInfo{}
	standalone := []ContainerInfo{}

	for _, container := range containers {
		project := strings.TrimSpace(
			container.ComposeProject,
		)

		if project == "" {
			standalone = append(
				standalone,
				container,
			)
			continue
		}

		projectGroups[project] = append(
			projectGroups[project],
			container,
		)
	}

	projectNames := make(
		[]string,
		0,
		len(projectGroups),
	)

	for project := range projectGroups {
		projectNames = append(
			projectNames,
			project,
		)
	}

	sort.Slice(
		projectNames,
		func(i, j int) bool {
			return strings.ToLower(
				projectNames[i],
			) <
				strings.ToLower(
					projectNames[j],
				)
		},
	)

	groups := make(
		[]trayContainerGroup,
		0,
		len(projectNames)+1,
	)

	for _, project := range projectNames {
		projectContainers := projectGroups[project]

		sortTrayGroupContainers(
			projectContainers,
		)

		groups = append(
			groups,
			trayContainerGroup{
				Name:       project,
				Project:    project,
				Standalone: false,
				Containers: projectContainers,
			},
		)
	}

	if len(standalone) > 0 {
		sortTrayGroupContainers(
			standalone,
		)

		groups = append(
			groups,
			trayContainerGroup{
				Name:       "Standalone Containers",
				Standalone: true,
				Containers: standalone,
			},
		)
	}

	return groups
}

func sortTrayGroupContainers(
	containers []ContainerInfo,
) {
	sort.SliceStable(
		containers,
		func(i, j int) bool {
			iRunning := containerIsRunning(
				containers[i],
			)

			jRunning := containerIsRunning(
				containers[j],
			)

			if iRunning != jRunning {
				return iRunning
			}

			return strings.ToLower(
				containerTrayItemName(
					containers[i],
				),
			) <
				strings.ToLower(
					containerTrayItemName(
						containers[j],
					),
				)
		},
	)
}

func containerTrayItemName(
	container ContainerInfo,
) string {
	if container.ComposeService != "" {
		return container.ComposeService
	}

	if container.Name != "" {
		return container.Name
	}

	if container.ShortID != "" {
		return container.ShortID
	}

	return container.ID
}

func containerIsRunning(
	container ContainerInfo,
) bool {
	return strings.EqualFold(
		strings.TrimSpace(
			container.State,
		),
		"running",
	)
}

func containerTrayName(
	container ContainerInfo,
) string {
	if container.ComposeProject != "" {
		if container.ComposeService != "" {
			return container.ComposeProject +
				"/" +
				container.ComposeService
		}

		return container.ComposeProject +
			"/" +
			container.Name
	}

	if container.Name != "" {
		return container.Name
	}

	if container.ShortID != "" {
		return container.ShortID
	}

	return container.ID
}

func countTrayContainers(
	containers []ContainerInfo,
) (
	running int,
	stopped int,
) {
	for _, container := range containers {
		if containerIsRunning(
			container,
		) {
			running++
		} else {
			stopped++
		}
	}

	return running, stopped
}

func shortTrayText(
	value string,
	max int,
) string {
	value = strings.TrimSpace(value)

	if max < 2 ||
		len([]rune(value)) <= max {
		return value
	}

	runes := []rune(value)

	return string(
		runes[:max-1],
	) + "…"
}

func joinTrayFailures(
	failures []string,
) error {
	if len(failures) == 0 {
		return nil
	}

	return fmt.Errorf(
		"%s",
		strings.Join(
			failures,
			"; ",
		),
	)
}
