<script setup lang="ts">
import { ws } from '../../api/ws'
import type { TaskStatus } from '../../api/tasks'
import { useEditorStore } from '../../stores/editor'
import StateBlock from '../StateBlock.vue'

const editor = useEditorStore()
const labels: Record<TaskStatus, string> = {
  preparing: '准备中', running: '运行中', completed: '已完成', cancelled: '已取消', failed: '失败',
}

function articleTitle(articleId: number): string {
  return editor.sessions.get(articleId)?.meta.title || `文章 #${articleId}`
}
</script>

<template>
  <div class="sk-tasks" role="tabpanel" aria-label="后台任务">
    <StateBlock v-if="ws.tasks.items.length === 0" state="empty" text="还没有后台任务。分析文章、推断段落类型或生成建议后，可在这里查看进度和取消任务。" />
    <ul v-else class="sk-task-list" aria-label="任务列表">
      <li v-for="task in ws.tasks.items" :key="task.id" class="sk-task">
        <div class="sk-task-content">
          <div>{{ task.title }} · {{ articleTitle(task.articleId) }}</div>
          <div class="text-caption" :role="task.status === 'failed' ? 'alert' : 'status'">{{ labels[task.status] }} · {{ new Date(task.startedAt).toLocaleTimeString() }}<template v-if="task.error && task.status === 'failed'"> · {{ task.error }}</template></div>
        </div>
        <div class="sk-task-actions">
          <v-btn v-if="task.open" size="small" @click="task.open()">{{ task.status === 'preparing' || task.status === 'running' ? '查看进度' : task.status === 'completed' ? '查看结果' : '查看详情' }}</v-btn>
          <v-btn v-if="task.status === 'preparing' || task.status === 'running'" size="small" @click="task.cancel()">取消任务</v-btn>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.sk-tasks {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.sk-task-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.sk-task {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 16px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--sk-line);
}
.sk-task-content {
  flex: 1 1 200px;
  min-width: 0;
  overflow-wrap: anywhere;
}
.sk-task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
@media (pointer: coarse) {
  .sk-task-actions :deep(.v-btn) {
    min-height: 44px;
  }
}
</style>
