<script setup lang="ts">
import {
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
} from 'vue'
import { Call, Events } from '@wailsio/runtime'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

interface Container {
  id: string
  name: string
  shortId: string
}

interface StreamOutputEvent {
  streamId: string
  containerId: string
  data?: string
  error?: string
  closed?: boolean
  seq: number
}

const props = defineProps<{
  container: Container
}>()

const emit = defineEmits<{
  close: []
}>()

const terminalHost = ref<HTMLElement | null>(null)
const status = ref<'connecting' | 'connected' | 'closed'>('connecting')

let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let sessionID = ''
let unsubscribeEvent: (() => void) | undefined
let resizeObserver: ResizeObserver | undefined
let inputDisposable: { dispose(): void } | undefined
let resizeDisposable: { dispose(): void } | undefined
let resizeTimer: ReturnType<typeof setTimeout> | undefined

async function start() {
  await nextTick()

  if (!terminalHost.value) {
    return
  }

  terminal = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    convertEol: false,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    fontSize: 13,
    lineHeight: 1.2,
    scrollback: 5000,
    allowProposedApi: false,
    theme: {
      background: '#000000',
      foreground: '#d4d4d8',
      cursor: '#e4e4e7',
      selectionBackground: '#3f3f46',
      black: '#18181b',
      red: '#f87171',
      green: '#4ade80',
      yellow: '#facc15',
      blue: '#60a5fa',
      magenta: '#c084fc',
      cyan: '#00e5ff',
      white: '#e4e4e7',
    },
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(terminalHost.value)
  fitAddon.fit()

  terminal.writeln('\x1b[90mDevStack: connecting to /bin/sh...\x1b[0m')

  unsubscribeEvent = Events.On(
    'devstack:terminal-output',
    (payload: any) => {
      const data = payload as StreamOutputEvent

      const matchesSession = sessionID
        ? data?.streamId === sessionID
        : data?.containerId === props.container.id

      if (!data || !matchesSession) {
        return
      }

      if (data.data) {
        terminal?.write(data.data)
      }

      if (data.error) {
        terminal?.writeln(`\r\n\x1b[31m[DevStack] ${data.error}\x1b[0m`)
      }

      if (data.closed) {
        status.value = 'closed'
        terminal?.writeln('\r\n\x1b[90m[session closed]\x1b[0m')
      }
    },
  )

  try {
    sessionID = await Call.ByName(
      'main.DockerService.StartTerminal',
      props.container.id,
    ) as string

    status.value = 'connected'

    inputDisposable = terminal.onData((data) => {
      if (!sessionID || status.value !== 'connected') {
        return
      }

      void Call.ByName(
        'main.DockerService.SendTerminalInput',
        sessionID,
        data,
      )
    })

    resizeDisposable = terminal.onResize(({ cols, rows }) => {
      scheduleResize(cols, rows)
    })

    resizeObserver = new ResizeObserver(() => {
      fitAddon?.fit()
    })

    resizeObserver.observe(terminalHost.value)

    fitAddon.fit()
    terminal.focus()

    scheduleResize(terminal.cols, terminal.rows)
  } catch (err) {
    status.value = 'closed'
    terminal.writeln(
      `\r\n\x1b[31m[DevStack] ${
        err instanceof Error ? err.message : String(err)
      }\x1b[0m`,
    )
  }
}

function scheduleResize(cols: number, rows: number) {
  if (!sessionID || status.value !== 'connected') {
    return
  }

  if (resizeTimer) {
    clearTimeout(resizeTimer)
  }

  resizeTimer = setTimeout(() => {
    void Call.ByName(
      'main.DockerService.ResizeTerminal',
      sessionID,
      cols,
      rows,
    )
  }, 80)
}

async function closeSession() {
  const currentSession = sessionID
  sessionID = ''
  status.value = 'closed'

  if (currentSession) {
    try {
      await Call.ByName(
        'main.DockerService.CloseTerminal',
        currentSession,
      )
    } catch {
      // Closing is best-effort.
    }
  }

  emit('close')
}

onMounted(start)

onBeforeUnmount(() => {
  if (resizeTimer) {
    clearTimeout(resizeTimer)
  }

  resizeObserver?.disconnect()
  unsubscribeEvent?.()
  inputDisposable?.dispose()
  resizeDisposable?.dispose()
  terminal?.dispose()

  const currentSession = sessionID
  sessionID = ''

  if (currentSession) {
    void Call.ByName(
      'main.DockerService.CloseTerminal',
      currentSession,
    )
  }
})
</script>

<template>
  <div
    class="terminal-backdrop fixed inset-0 z-[70] flex items-center justify-center p-5"
    @click.self="closeSession"
  >
    <section
      class="terminal-shell flex h-[88vh] w-full max-w-7xl flex-col overflow-hidden bg-black"
    >
      <header
        class="terminal-header flex items-center justify-between px-4 py-3"
      >
        <div>
          <div class="flex items-center gap-2">
            <strong class="font-mono text-sm text-zinc-200">
              {{ container.name || container.shortId }}
            </strong>

            <span
              class="rounded-full px-2 py-0.5 text-[10px]"
              :class="
                status === 'connected'
                  ? 'bg-emerald-500/10 text-emerald-400'
                  : status === 'connecting'
                    ? 'bg-amber-500/10 text-amber-400'
                    : 'bg-red-500/10 text-red-400'
              "
            >
              {{ status }}
            </span>
          </div>

          <div class="mt-0.5 text-[11px] text-zinc-600">
            Full Docker TTY · /bin/sh
          </div>
        </div>

        <button
          class="rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800"
          @click="closeSession"
        >
          Close
        </button>
      </header>

      <div
        ref="terminalHost"
        class="min-h-0 flex-1 overflow-hidden p-2"
      />
    </section>
  </div>
</template>

<style scoped>
.terminal-backdrop { background:rgb(3 4 8/.84); backdrop-filter:blur(14px); }
.terminal-shell { border:1px solid rgb(255 255 255/.11); border-radius:1.15rem; box-shadow:0 30px 90px rgb(0 0 0/.6),inset 0 1px rgb(255 255 255/.05); }
.terminal-header { border-bottom:1px solid rgb(255 255 255/.065); background:linear-gradient(90deg,#151621,#11121b); }
:deep(.xterm) {
  height: 100%;
}

:deep(.xterm-viewport) {
  scrollbar-color: #3f3f46 #09090b;
}
</style>
