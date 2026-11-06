<script setup lang="ts">
import { mdiChevronRight } from '@mdi/js'
import { useEditorStore } from '../../stores/editor'
import { useUiStore, type InspectorTab } from '../../stores/ui'
import StateBlock from '../StateBlock.vue'
import DescDistribution from './DescDistribution.vue'
import StructureView from './StructureView.vue'
import SuggestionList from './SuggestionList.vue'

const ui = useUiStore()
const editor = useEditorStore()

const tabs: { id: InspectorTab; label: string }[] = [
  { id: 'suggestions', label: '建议' },
  { id: 'distribution', label: '描写分布' },
  { id: 'structure', label: '全文结构' },
]
</script>

<template>
  <aside class="sk-inspector bg-sk-panel" aria-label="检查器">
    <div class="sk-insp-tabs">
      <v-tabs v-model="ui.inspectorTab" density="comfortable" color="primary" class="flex-grow-1" aria-label="检查器视图">
      <v-tab
        v-for="t in tabs"
        :key="t.id"
        :value="t.id"
      >
        {{ t.label }}
      </v-tab>
      </v-tabs>
      <v-btn :icon="mdiChevronRight" size="x-small" density="comfortable" aria-label="收起检查器（Ctrl+Alt+B）" title="收起（Ctrl+Alt+B）" @click="ui.layout.inspectorOpen = false" />
    </div>
    <div class="sk-insp-body" role="tabpanel">
      <Transition name="sk-view" mode="out-in">
        <StateBlock v-if="!editor.activeSession" state="empty" text="打开一篇文章后，这里显示它的待评审建议、描写分布和全文结构。" />
        <div v-else :key="`${editor.activeSession.articleId}:${ui.inspectorTab}`">
          <SuggestionList v-if="ui.inspectorTab === 'suggestions'" :key="`s${editor.activeSession.articleId}`" :session="editor.activeSession" />
          <DescDistribution v-else-if="ui.inspectorTab === 'distribution'" :session="editor.activeSession" />
          <StructureView v-else :session="editor.activeSession" />
        </div>
      </Transition>
    </div>
  </aside>
</template>

<style scoped>
.sk-inspector {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}
.sk-insp-tabs {
  flex: none;
  display: flex;
  align-items: center;
  min-height: 48px;
  padding: 0 4px 0 6px;
  border-bottom: 1px solid var(--sk-line);
}
.sk-itab {
  height: 36px;
  padding: 0 10px;
  font: inherit;
  font-size: 12px;
  background: none;
  border: 0;
  color: var(--sk-muted);
  cursor: pointer;
}
.sk-itab-on {
  color: rgb(var(--v-theme-on-surface));
  box-shadow: inset 0 -2px 0 rgb(var(--v-theme-primary));
}
.sk-itab:focus-visible {
  outline-offset: -3px;
}
.sk-fill {
  flex: 1;
}
.sk-insp-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
</style>
