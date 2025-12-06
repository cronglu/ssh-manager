<template>
  <div class="container">
    <header style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px;">
      <h1>SSH Manager</h1>
      <button class="btn-primary" @click="openModal()">Add Server</button>
    </header>

    <ServerList :servers="servers" @refresh="loadServers" @connect="onConnect" @delete="onDelete" @edit="openModal" />

    <AddServerModal v-if="showModal" @close="showModal = false" @saved="loadServers" :servers="servers" :initial-server="editingServer" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import ServerList from './components/ServerList.vue'
import AddServerModal from './components/AddServerModal.vue'
import { GetServers, DeleteServer, Connect } from '../wailsjs/go/main/App'

const servers = ref([])
const showModal = ref(false)
const editingServer = ref(null)

const loadServers = async () => {
    try {
        servers.value = await GetServers() || []
    } catch (e) {
        console.error("Failed to load servers:", e)
    }
}

const onConnect = async (name) => {
    try {
        await Connect(name)
    } catch (e) {
        alert("Failed to connect: " + e)
    }
}

const onDelete = async (name) => {
    if (confirm(`Are you sure you want to delete server '${name}'?`)) {
        try {
            await DeleteServer(name)
            await loadServers()
        } catch (e) {
            alert("Failed to delete: " + e)
        }
    }
}

const openModal = (server = null) => {
    editingServer.value = server // If null, it's add mode
    showModal.value = true
}

onMounted(() => {
    loadServers()
})
</script>
