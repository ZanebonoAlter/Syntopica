<script setup lang="ts">
import { Icon } from '@iconify/vue'

/**
 * FeedEmptyGuide — RSS 源空状态引导卡片。
 *
 * 无 Feed 且无分类时由父组件条件渲染（`v-if`），展示引导文案与「添加 RSS 源」操作。
 * 点击操作按钮 emit('add')，由父组件打开添加订阅源对话框。
 *
 * 详见 openspec/changes/user-onboarding/spec.md「Empty state guide for feeds」。
 */
defineEmits<{
  add: []
}>()
</script>

<template>
  <div class="feed-empty-guide">
    <Icon icon="mdi:rss" width="40" height="40" class="feed-empty-guide__icon" />
    <p class="feed-empty-guide__title">添加你的第一个 RSS 源</p>
    <p class="feed-empty-guide__desc">订阅博客、新闻或任意 RSS 地址，文章会自动抓取并归类。</p>
    <button type="button" class="feed-empty-guide__btn" @click="$emit('add')">
      <Icon icon="mdi:plus" width="16" height="16" />
      添加 RSS 源
    </button>
  </div>
</template>

<style scoped>
.feed-empty-guide {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2rem 1.25rem;
  text-align: center;
}
.feed-empty-guide__icon {
  color: var(--color-text-muted);
  opacity: 0.5;
}
.feed-empty-guide__title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
}
.feed-empty-guide__desc {
  font-size: 0.78rem;
  color: var(--color-text-muted);
  margin: 0;
  line-height: 1.5;
}
.feed-empty-guide__btn {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.5rem;
  padding: 0.45rem 0.9rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  background: var(--color-accent-subtle, transparent);
  color: var(--color-accent);
  font-size: 0.8rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.12s ease;
}
.feed-empty-guide__btn:hover {
  background: var(--color-bg-hover);
  border-color: var(--color-accent);
}

/* 窄屏（<768px）：按钮全宽、收紧内边距（mobile-viewport-stage1 任务 4.1） */
@media (max-width: 767.98px) {
  .feed-empty-guide {
    padding: 1.5rem 1rem;
  }

  .feed-empty-guide__btn {
    width: 100%;
    justify-content: center;
  }
}
</style>
