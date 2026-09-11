<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Call } from '@wailsio/runtime'

interface Container {
  id: string
  name: string
  shortId: string
}

interface MountDetail {
  type: string
  name?: string
  source: string
  destination: string
  rw: boolean
}

interface NetworkDetail {
  name: string
  ipAddress: string
  gateway: string
  mac: string
}

interface ContainerDetails {
  id: string
  name: string
  image: string
  created: string
  path: string
  args: string[]
  entrypoint: string[]
  command: string[]
  workingDir: string
  user: string
  environment: string[]
  mounts: MountDetail[]
  networks: NetworkDetail[]
  labels: Record<string, string>
  restartPolicy: string
  state: string
  exitCode: number
  startedAt: string
  finishedAt: string
}

const props = defineProps<{
  container: Container
}>()

const emit = defineEmits<{
  close: []
}>()

const loading = ref(true)
const error = ref('')
const details = ref<ContainerDetails | null>(null)
const search = ref('')
const envVisible = ref(false)

const filteredEnv = computed(() => {
  const query = search.value.trim().toLowerCase()
  const items = details.value?.environment ?? []
  return items
    .filter((item) => !query || item.toLowerCase().includes(query))
    .map((item) => {
      const separator = item.indexOf('=')
      return {
        raw: item,
        key: separator < 0 ? item : item.slice(0, separator),
        value: separator < 0 ? '' : item.slice(separator + 1),
      }
    })
})

const sortedLabels = computed(() => {
  return Object.entries(details.value?.labels ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
})

function displayEnvValue(value: string) {
  return envVisible.value ? value : '••••••••'
}

function formatTimestamp(value: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const timestamp = new Date(value)
  return Number.isNaN(timestamp.getTime()) ? value : timestamp.toLocaleString()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', handleKeydown)
  try {
    details.value = await Call.ByName(
      'main.DockerService.GetContainerDetails',
      props.container.id,
    ) as ContainerDetails
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div
    class="modal-backdrop fixed inset-0 z-[65] flex items-center justify-center p-5"
    @click.self="emit('close')"
  >
    <section
      class="modal-shell flex h-[88vh] w-full max-w-6xl flex-col overflow-hidden"
    >
      <header
        class="modal-header flex items-center justify-between px-5 py-4"
      >
        <div class="flex min-w-0 items-center gap-3">
          <div class="container-glyph" aria-hidden="true"><span /><span /><span /></div>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <h3 class="truncate font-semibold">
                {{ container.name || container.shortId }}
              </h3>
              <span v-if="details" class="state-pill" :class="details.state === 'running' ? 'running' : 'stopped'">
                <i />{{ details.state || 'unknown' }}
              </span>
            </div>
            <p class="mt-0.5 truncate font-mono text-[11px] text-zinc-500">
              {{ container.shortId }} · Container details
            </p>
          </div>
        </div>

        <button class="close-button" aria-label="Close container details" title="Close (Esc)" @click="emit('close')">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg>
        </button>
      </header>

      <div
        v-if="loading"
        class="flex flex-1 items-center justify-center text-sm text-zinc-500"
      >
        Loading inspect data…
      </div>

      <div
        v-else-if="error"
        class="m-5 rounded-lg border border-red-900 bg-red-950/40 p-4 text-sm text-red-300"
      >
        {{ error }}
      </div>

      <div
        v-else-if="details"
        class="flex-1 overflow-auto p-5"
      >
        <div class="summary-grid">
          <div class="info-card">
            <span class="label">State</span>
            <strong class="capitalize">{{ details.state || 'unknown' }}</strong>
          </div>
          <div class="info-card">
            <span class="label">Restart policy</span>
            <strong>{{ details.restartPolicy || 'no' }}</strong>
          </div>
          <div class="info-card">
            <span class="label">User</span>
            <strong class="font-mono">{{ details.user || 'default' }}</strong>
          </div>
          <div class="info-card">
            <span class="label">Exit code</span>
            <strong>{{ details.exitCode }}</strong>
          </div>
        </div>

        <section class="detail-section">
          <div class="section-heading"><span class="section-icon violet">⌘</span><h4>Process</h4></div>
          <dl class="detail-grid">
            <dt>Image</dt>
            <dd>{{ details.image }}</dd>
            <dt>Working directory</dt>
            <dd>{{ details.workingDir || '/' }}</dd>
            <dt>Entrypoint</dt>
            <dd>{{ details.entrypoint?.join(' ') || '—' }}</dd>
            <dt>Command</dt>
            <dd>{{ details.command?.join(' ') || '—' }}</dd>
            <dt>Created</dt>
            <dd :title="details.created">{{ formatTimestamp(details.created) }}</dd>
            <dt>Started</dt>
            <dd :title="details.startedAt">{{ formatTimestamp(details.startedAt) }}</dd>
          </dl>
        </section>

        <section class="detail-section">
          <div class="section-toolbar">
            <div class="section-heading"><span class="section-icon cyan">≡</span><h4>Environment</h4><span class="section-count">{{ details.environment?.length || 0 }}</span></div>

            <div class="section-controls">
              <input
                v-model="search"
                class="field w-56"
                placeholder="Filter environment…"
                aria-label="Filter environment variables"
              />
              <button class="small-button" @click="envVisible = !envVisible">
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12s3.5-5.5 9.5-5.5 9.5 5.5 9.5 5.5-3.5 5.5-9.5 5.5S2.5 12 2.5 12Z" /><circle cx="12" cy="12" r="2.5" /><path v-if="envVisible" d="m4 4 16 16" /></svg>
                {{ envVisible ? 'Mask values' : 'Show values' }}
              </button>
            </div>
          </div>

          <div class="code-list">
            <div
              v-for="item in filteredEnv"
              :key="item.raw"
              class="env-row"
            >
              <span class="env-key">{{ item.key }}</span><span class="env-equals">=</span><span class="env-value" :class="{ masked: !envVisible }">{{ displayEnvValue(item.value) }}</span>
            </div>

            <div
              v-if="!filteredEnv.length"
              class="px-3 py-4 text-zinc-600"
            >
              No matching environment variables.
            </div>
          </div>
        </section>

        <section class="detail-section">
          <div class="section-heading"><span class="section-icon pink">◇</span><h4>Mounts</h4><span class="section-count">{{ details.mounts?.length || 0 }}</span></div>

          <div v-if="details.mounts?.length" class="data-table mt-3 overflow-hidden rounded-lg border border-zinc-800">
            <table class="w-full text-sm">
              <thead class="bg-zinc-900 text-left text-xs text-zinc-500">
                <tr>
                  <th class="px-3 py-2">TYPE</th>
                  <th class="px-3 py-2">SOURCE</th>
                  <th class="px-3 py-2">DESTINATION</th>
                  <th class="px-3 py-2">MODE</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="mount in details.mounts"
                  :key="`${mount.source}-${mount.destination}`"
                  class="border-t border-zinc-800"
                >
                  <td class="px-3 py-2">{{ mount.type }}</td>
                  <td class="px-3 py-2 font-mono text-xs text-zinc-400">
                    {{ mount.name || mount.source }}
                  </td>
                  <td class="px-3 py-2 font-mono text-xs text-zinc-300">
                    {{ mount.destination }}
                  </td>
                  <td class="px-3 py-2">{{ mount.rw ? 'rw' : 'ro' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="empty-detail">No mounts attached to this container.</div>
        </section>

        <section class="detail-section">
          <div class="section-heading"><span class="section-icon green">⌁</span><h4>Networks</h4><span class="section-count">{{ details.networks?.length || 0 }}</span></div>

          <div class="mt-3 grid gap-3 md:grid-cols-2">
            <div
              v-for="network in details.networks"
              :key="network.name"
              class="network-card rounded-lg border border-zinc-800 bg-zinc-900/50 p-3"
            >
              <strong>{{ network.name }}</strong>
              <div class="mt-2 font-mono text-xs leading-5 text-zinc-500">
                IP: {{ network.ipAddress || '—' }}<br />
                Gateway: {{ network.gateway || '—' }}<br />
                MAC: {{ network.mac || '—' }}
              </div>
            </div>
          </div>
          <div v-if="!details.networks?.length" class="empty-detail">No network attachments found.</div>
        </section>

        <section class="detail-section">
          <div class="section-heading"><span class="section-icon amber">#</span><h4>Labels</h4><span class="section-count">{{ sortedLabels.length }}</span></div>

          <div v-if="sortedLabels.length" class="code-list mt-3">
            <div
              v-for="[key, value] in sortedLabels"
              :key="key"
              class="border-b border-zinc-800 px-3 py-2 last:border-none"
            >
              <span class="text-sky-400">{{ key }}</span>
              <span class="text-zinc-600">=</span>
              <span class="break-all text-zinc-400">{{ value }}</span>
            </div>
          </div>
          <div v-else class="empty-detail">No labels applied to this container.</div>
        </section>
      </div>
    </section>
  </div>
</template>

<style scoped>
.modal-backdrop { background:rgb(3 4 8/.82); backdrop-filter:blur(14px); }
.modal-shell {
  --detail-border: rgb(255 255 255/.075);
  --detail-border-strong: rgb(255 255 255/.11);
  --detail-surface: rgb(255 255 255/.025);
  --detail-surface-raised: rgb(255 255 255/.04);
  --detail-code: rgb(6 7 12/.45);
  --detail-text: #e4e4e9;
  --detail-muted: #858591;
  --detail-faint: #666674;
  border:1px solid rgb(255 255 255/.1);
  border-radius:1.15rem;
  background:linear-gradient(145deg,#151621,#0e0f17);
  color:var(--detail-text);
  box-shadow:0 30px 90px rgb(0 0 0/.55),inset 0 1px rgb(255 255 255/.05);
}
.modal-header { border-bottom:1px solid rgb(255 255 255/.06); background:rgb(255 255 255/.018); }
.container-glyph { position:relative; display:grid; width:2.5rem; height:2.5rem; flex:none; place-items:center; border:1px solid rgb(0 229 255/.28); border-radius:.8rem; background:linear-gradient(145deg,rgb(10 104 255/.2),rgb(0 229 255/.07)); }
.container-glyph span { position:absolute; width:1.15rem; height:.3rem; border:1px solid #29f3ff; border-radius:.18rem; transform:skewY(-18deg); }
.container-glyph span:first-child { translate:0 -.38rem; opacity:.5; }
.container-glyph span:last-child { translate:0 .38rem; opacity:.72; }
.state-pill { display:inline-flex; align-items:center; gap:.35rem; border:1px solid var(--detail-border); border-radius:999px; padding:.14rem .48rem; font-size:.58rem; font-weight:650; letter-spacing:.03em; text-transform:uppercase; }
.state-pill i { width:.36rem; height:.36rem; border-radius:999px; background:currentColor; box-shadow:0 0 7px currentColor; }
.state-pill.running { border-color:rgb(65 205 151/.18); background:rgb(48 201 144/.08); color:#55dca7; }
.state-pill.stopped { color:var(--detail-muted); }
.close-button { display:grid; width:2.25rem; height:2.25rem; flex:none; place-items:center; border:1px solid var(--detail-border-strong); border-radius:.68rem; background:var(--detail-surface-raised); color:var(--detail-muted); transition:160ms ease; }
.close-button:hover { border-color:rgb(0 229 255/.35); background:rgb(10 104 255/.1); color:#29f3ff; transform:translateY(-1px); }
.close-button svg { width:1rem; height:1rem; fill:none; stroke:currentColor; stroke-width:1.8; stroke-linecap:round; }
.small-button {
  display:inline-flex;
  align-items:center;
  gap:.4rem;
  border: 1px solid var(--detail-border-strong);
  border-radius: .65rem;
  background: var(--detail-surface-raised);
  padding: 0.4rem 0.7rem;
  font-size: 0.75rem;
  color: var(--detail-text);
}

.small-button:hover { border-color:rgb(0 229 255/.32); background:rgb(10 104 255/.09); }
.small-button svg { width:.88rem; height:.88rem; fill:none; stroke:currentColor; stroke-width:1.6; stroke-linecap:round; stroke-linejoin:round; }

.field {
  border: 1px solid var(--detail-border-strong);
  border-radius: .65rem;
  background: var(--detail-code);
  padding: 0.4rem 0.65rem;
  font-size: 0.75rem;
  color: var(--detail-text);
  outline: none;
}
.field:focus { border-color:rgb(0 229 255/.65); box-shadow:0 0 0 3px rgb(10 104 255/.12); }

.summary-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:.75rem; }
.info-card {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border: 1px solid var(--detail-border);
  border-radius: .8rem;
  background: var(--detail-surface);
  padding: 0.9rem;
}
.info-card strong { overflow:hidden; color:var(--detail-text); font-size:.82rem; text-overflow:ellipsis; white-space:nowrap; }

.label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--detail-faint);
}

.detail-section {
  margin-top: 1.5rem;
}

.detail-section h4 {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--detail-text);
}
.section-heading { display:flex; min-width:0; align-items:center; gap:.55rem; }
.section-icon { display:grid; width:1.65rem; height:1.65rem; flex:none; place-items:center; border-radius:.5rem; font-size:.72rem; font-weight:700; }
.section-icon.violet { background:rgb(10 104 255/.12); color:#29f3ff; }
.section-icon.cyan { background:rgb(34 191 211/.1); color:#54d3e3; }
.section-icon.pink { background:rgb(230 84 157/.1); color:#ea75b1; }
.section-icon.green { background:rgb(43 200 142/.1); color:#58dba8; }
.section-icon.amber { background:rgb(226 155 58/.1); color:#eeb66d; }
.section-count { border:1px solid var(--detail-border); border-radius:999px; background:var(--detail-surface); padding:.08rem .42rem; color:var(--detail-faint); font-size:.58rem; }
.section-toolbar { display:flex; align-items:center; justify-content:space-between; gap:1rem; margin-bottom:.75rem; }
.section-controls { display:flex; gap:.5rem; }

.detail-grid {
  display: grid;
  grid-template-columns: 150px 1fr;
  margin-top: 0.75rem;
  overflow: hidden;
  border: 1px solid var(--detail-border);
  border-radius: .75rem;
  font-size: 0.75rem;
}

.detail-grid dt,
.detail-grid dd {
  border-bottom: 1px solid var(--detail-border);
  padding: 0.6rem 0.75rem;
}
.detail-grid dt:nth-last-of-type(1),
.detail-grid dd:nth-last-of-type(1) { border-bottom:0; }

.detail-grid dt {
  background: var(--detail-surface);
  color: var(--detail-muted);
}

.detail-grid dd {
  min-width: 0;
  overflow-wrap: anywhere;
  font-family: ui-monospace, monospace;
  color: var(--detail-muted);
}

.code-list {
  overflow: hidden;
  border: 1px solid var(--detail-border);
  border-radius: .75rem;
  background: var(--detail-code);
  font-family: ui-monospace, monospace;
  font-size: 0.72rem;
  line-height: 1.25rem;
  color: var(--detail-muted);
}
.env-row { display:grid; grid-template-columns:minmax(9rem,.42fr) auto minmax(0,1fr); gap:.35rem; border-bottom:1px solid var(--detail-border); padding:.56rem .75rem; }
.env-row:last-child { border-bottom:0; }
.env-key { overflow:hidden; color:#29f3ff; font-weight:600; text-overflow:ellipsis; white-space:nowrap; }
.env-equals { color:var(--detail-faint); }
.env-value { min-width:0; overflow-wrap:anywhere; color:var(--detail-muted); }
.env-value.masked { color:var(--detail-faint); letter-spacing:.08em; }
.data-table { border-color:var(--detail-border)!important; }
.data-table thead { background:var(--detail-surface-raised)!important; }
.data-table tr { border-color:var(--detail-border)!important; }
.network-card { border-color:var(--detail-border)!important; background:var(--detail-surface)!important; }
.empty-detail { margin-top:.75rem; border:1px dashed var(--detail-border-strong); border-radius:.75rem; padding:1.2rem; color:var(--detail-faint); font-size:.75rem; text-align:center; }

:global(html[data-theme="light"] .modal-shell) {
  --detail-border: rgb(38 39 52/.09);
  --detail-border-strong: rgb(38 39 52/.14);
  --detail-surface: rgb(51 48 69/.035);
  --detail-surface-raised: rgb(255 255 255/.76);
  --detail-code: rgb(241 242 247/.82);
  --detail-text: #292a34;
  --detail-muted: #60616e;
  --detail-faint: #7b7c88;
}
:global(html[data-theme="light"] .modal-shell .modal-header) { border-color:var(--detail-border); background:rgb(255 255 255/.62); }
:global(html[data-theme="light"] .modal-shell .container-glyph) { background:linear-gradient(145deg,rgb(10 104 255/.13),rgb(0 229 255/.06)); }
:global(html[data-theme="light"] .modal-shell .state-pill.running) { color:#13885d; }
:global(html[data-theme="light"] .modal-shell .section-icon.violet) { color:#063e9b; }
:global(html[data-theme="light"] .modal-shell .section-icon.cyan) { color:#147f92; }
:global(html[data-theme="light"] .modal-shell .section-icon.pink) { color:#bd3d7d; }
:global(html[data-theme="light"] .modal-shell .section-icon.green) { color:#14845b; }
:global(html[data-theme="light"] .modal-shell .section-icon.amber) { color:#9b681f; }
:global(html[data-theme="light"] .modal-shell .env-key) { color:#063e9b; }

@media (max-width: 760px) {
  .summary-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }
  .section-toolbar { align-items:flex-start; flex-direction:column; }
  .section-controls { width:100%; }
  .section-controls .field { min-width:0; width:100%; }
  .detail-grid { grid-template-columns:7rem minmax(0,1fr); }
  .env-row { grid-template-columns:minmax(6.5rem,.42fr) auto minmax(0,1fr); }
}
</style>
