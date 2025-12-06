<template>
    <div class="modal-overlay" @click.self="$emit('close')">
        <div class="modal-content">
            <div class="modal-header">
                <h3>{{ isEdit ? 'Edit Server' : 'Add New Server' }}</h3>
                <button @click="$emit('close')" style="background: none; color: white; font-size: 1.2em;">&times;</button>
            </div>
            
            <form @submit.prevent="submit">
                <div class="form-group">
                    <label>Name</label>
                    <input v-model="form.name" required placeholder="e.g. prod-db-01" />
                </div>
                
                <div class="form-group">
                    <label>Group</label>
                    <input v-model="form.group" placeholder="e.g. Production" />
                </div>
                
                <div class="form-group">
                    <label>Host</label>
                    <input v-model="form.host" required placeholder="IP or Hostname" />
                </div>
                
                <div class="form-group">
                    <label>User</label>
                    <input v-model="form.user" required placeholder="ssh user" />
                </div>
                
                <div class="form-group">
                    <label>Password</label>
                    <input v-model="password" type="password" :placeholder="passwordPlaceholder" />
                </div>
                
                <div class="form-group">
                    <label>Bastion (Jump Host)</label>
                    <select v-model="form.bastion">
                        <option value="">None</option>
                        <option v-for="s in potentialBastions" :key="s.name" :value="s.name">
                            {{ s.name }} ({{ s.host }})
                        </option>
                    </select>
                </div>

                <div class="modal-footer">
                    <button type="button" class="btn-secondary" @click="$emit('close')">Cancel</button>
                    <button type="submit" class="btn-primary">{{ isEdit ? 'Save Changes' : 'Add Server' }}</button>
                </div>
            </form>
        </div>
    </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { SaveServer } from '../../wailsjs/go/main/App'

const props = defineProps({
    servers: Array,
    initialServer: {
        type: Object,
        default: null
    }
})

const emit = defineEmits(['close', 'saved'])

const form = ref({
    name: '',
    group: 'Default',
    host: '',
    user: 'root',
    bastion: ''
})
const password = ref('')

const isEdit = computed(() => !!props.initialServer)
const passwordPlaceholder = computed(() => isEdit.value ? '(Unchanged if empty)' : '(Optional) Stored in Keychain')

const potentialBastions = computed(() => {
    return props.servers.filter(s => s.name !== form.value.name)
})

const initForm = () => {
    if (props.initialServer) {
        form.value = { ...props.initialServer }
        password.value = ''
    } else {
        form.value = {
            name: '',
            group: 'Default',
            host: '',
            user: 'root',
            bastion: ''
        }
        password.value = ''
    }
}

onMounted(initForm)
// Also watch if modal is reused? 
// The parent uses v-if="showModal", so component is re-created each time. 
// But if v-show, we need watcher.
// In App.vue it is `v-if="showModal"`, so onMounted is sufficient.

const submit = async () => {
    try {
        const oldName = isEdit.value ? props.initialServer.name : ""
        await SaveServer(form.value, oldName, password.value)
        emit('saved')
        emit('close')
    } catch (e) {
        alert("Error saving server: " + e)
    }
}
</script>
