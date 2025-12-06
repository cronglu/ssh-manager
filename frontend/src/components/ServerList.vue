<template>
    <div class="server-list">
        <div v-if="servers.length === 0" style="padding: 40px; color: #565f89;">
            No servers configured. Click "Add Server" to get started.
        </div>

        <table v-else class="server-table">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Host</th>
                    <th>User</th>
                    <th>Bastion</th>
                    <th style="text-align: right;">Actions</th>
                </tr>
            </thead>
            <tbody>
                <template v-for="(group, groupName) in groupedServers" :key="groupName">
                   <tr class="group-header" @click="toggleGroup(groupName)" style="cursor: pointer;">
                       <td colspan="5">
                           <span style="font-size: 0.8em; margin-right: 8px;">
                               {{ collapsed[groupName] ? '▶' : '▼' }}
                           </span>
                           {{ groupName || 'Ungrouped' }}
                           <span style="font-size: 0.8em; color: #565f89; margin-left: 8px;">
                               ({{ group.length }})
                           </span>
                       </td>
                   </tr>
                   <template v-if="!collapsed[groupName]">
                       <tr v-for="server in group" :key="server.name">
                           <td>{{ server.name }}</td>
                           <td>{{ server.host }}</td>
                           <td>{{ server.user }}</td>
                           <td>{{ server.bastion || '-' }}</td>
                           <td style="text-align: right;">
                               <button class="btn-primary" style="margin-right: 8px; padding: 4px 12px; font-size: 0.9em;" @click="$emit('connect', server.name)">Connect</button>
                               <button class="btn-secondary" style="margin-right: 8px; padding: 4px 12px; font-size: 0.9em;" @click="$emit('edit', server)">Edit</button>
                               <button class="btn-danger" style="padding: 4px 12px; font-size: 0.9em;" @click="$emit('delete', server.name)">Delete</button>
                           </td>
                       </tr>
                   </template>
                </template>
            </tbody>
        </table>
    </div>
</template>

<script setup>
import { computed, ref } from 'vue'

const props = defineProps({
    servers: {
        type: Array,
        default: () => []
    }
})

const collapsed = ref({})

const toggleGroup = (groupName) => {
    collapsed.value[groupName] = !collapsed.value[groupName]
}

const groupedServers = computed(() => {
    const groups = {}
    props.servers.forEach(s => {
        const g = s.group || 'Default'
        if (!groups[g]) groups[g] = []
        groups[g].push(s)
    })
    // Sort keys alphabetically
    return Object.keys(groups).sort().reduce(
      (obj, key) => { 
        obj[key] = groups[key]; 
        return obj;
      }, 
      {}
    );
})

defineEmits(['refresh', 'connect', 'delete', 'edit'])
</script>
