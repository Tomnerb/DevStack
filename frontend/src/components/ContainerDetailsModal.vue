<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
const envVisible = ref(true)

const filteredEnv = computed(() => {
  const query = search.value.trim().toLowerCase()
  const items = details.value?.environment ?? []

  if (!query) {
    return items
  }

  return items.filter((item) => item.toLowerCase().includes(query))
})

const sortedLabels = computed(() => {
  return Object.entries(details.value?.labels ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
})

function maskEnv(value: string) {
  if (envVisible.value) {
    return value
  }

  const index = value.indexOf('=')
  if (index < 0) {
    return value
  }

  return `${value.slice(0, index + 1)}••••••••`
}

onMounted(async () => {
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
        <div>
          <h3 class="font-semibold">
            {{ container.name || container.shortId }}
          </h3>
          <p class="mt-0.5 text-xs text-zinc-500">Container details</p>
        </div>

        <button class="small-button" @click="emit('close')">Close</button>
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
        <div class="grid gap-4 md:grid-cols-3">
          <div class="info-card">
            <span class="label">State</span>
            <strong>{{ details.state }}</strong>
          </div>
          <div class="info-card">
            <span class="label">Restart policy</span>
            <strong>{{ details.restartPolicy || 'no' }}</strong>
          </div>
          <div class="info-card">
            <span class="label">User</span>
            <strong class="font-mono">{{ details.user || 'default' }}</strong>
          </div>
        </div>

        <section class="detail-section">
          <h4>Process</h4>
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
            <dd>{{ details.created }}</dd>
            <dt>Started</dt>
            <dd>{{ details.startedAt }}</dd>
          </dl>
        </section>

        <section class="detail-section">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h4>Environment ({{ details.environment?.length || 0 }})</h4>

            <div class="flex gap-2">
              <input
                v-model="search"
                class="field w-56"
                placeholder="Filter environment…"
              />
              <button class="small-button" @click="envVisible = !envVisible">
                {{ envVisible ? 'Mask values' : 'Show values' }}
              </button>
            </div>
          </div>

          <div class="code-list">
            <div
              v-for="item in filteredEnv"
              :key="item"
              class="break-all border-b border-zinc-800 px-3 py-2 last:border-none"
            >
              {{ maskEnv(item) }}
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
          <h4>Mounts ({{ details.mounts?.length || 0 }})</h4>

          <div class="mt-3 overflow-hidden rounded-lg border border-zinc-800">
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
        </section>

        <section class="detail-section">
          <h4>Networks</h4>

          <div class="mt-3 grid gap-3 md:grid-cols-2">
            <div
              v-for="network in details.networks"
              :key="network.name"
              class="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3"
            >
              <strong>{{ network.name }}</strong>
              <div class="mt-2 font-mono text-xs leading-5 text-zinc-500">
                IP: {{ network.ipAddress || '—' }}<br />
                Gateway: {{ network.gateway || '—' }}<br />
                MAC: {{ network.mac || '—' }}
              </div>
            </div>
          </div>
        </section>

        <section class="detail-section">
          <h4>Labels ({{ sortedLabels.length }})</h4>

          <div class="code-list mt-3">
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
        </section>
      </div>
    </section>
  </div>
</template>

<style scoped>
.modal-backdrop { background:rgb(3 4 8/.82); backdrop-filter:blur(14px); }
.modal-shell { border:1px solid rgb(255 255 255/.1); border-radius:1.15rem; background:linear-gradient(145deg,#151621,#0e0f17); box-shadow:0 30px 90px rgb(0 0 0/.55),inset 0 1px rgb(255 255 255/.05); }
.modal-header { border-bottom:1px solid rgb(255 255 255/.06); background:rgb(255 255 255/.018); }
.small-button {
  border: 1px solid rgb(255 255 255/.09);
  border-radius: .65rem;
  background: rgb(255 255 255/.04);
  padding: 0.4rem 0.7rem;
  font-size: 0.75rem;
  color: rgb(212 212 216);
}

.small-button:hover {
  background: rgb(255 255 255/.075);
}

.field {
  border: 1px solid rgb(255 255 255/.085);
  border-radius: .65rem;
  background: rgb(6 7 12/.48);
  padding: 0.4rem 0.65rem;
  font-size: 0.75rem;
  color: rgb(228 228 231);
  outline: none;
}

.info-card {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  border: 1px solid rgb(255 255 255/.065);
  border-radius: .8rem;
  background: rgb(255 255 255/.025);
  padding: 0.9rem;
}

.label {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: rgb(113 113 122);
}

.detail-section {
  margin-top: 1.5rem;
}

.detail-section h4 {
  font-size: 0.85rem;
  font-weight: 600;
  color: rgb(228 228 231);
}

.detail-grid {
  display: grid;
  grid-template-columns: 150px 1fr;
  margin-top: 0.75rem;
  overflow: hidden;
  border: 1px solid rgb(255 255 255/.065);
  border-radius: .75rem;
  font-size: 0.75rem;
}

.detail-grid dt,
.detail-grid dd {
  border-bottom: 1px solid rgb(39 39 42);
  padding: 0.6rem 0.75rem;
}

.detail-grid dt {
  background: rgb(255 255 255/.025);
  color: rgb(113 113 122);
}

.detail-grid dd {
  min-width: 0;
  overflow-wrap: anywhere;
  font-family: ui-monospace, monospace;
  color: rgb(161 161 170);
}

.code-list {
  overflow: hidden;
  border: 1px solid rgb(255 255 255/.065);
  border-radius: .75rem;
  background: rgb(6 7 12/.45);
  font-family: ui-monospace, monospace;
  font-size: 0.72rem;
  line-height: 1.25rem;
  color: rgb(161 161 170);
}
</style>
