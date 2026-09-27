/**
 * 路由切换加载反馈接线（fix-spa-nav-loading-ux D1）
 *
 * router.beforeEach 启动 250ms 延迟计时；router.afterEach（成功与失败——
 * 守卫取消/重定向以 NavigationFailure 收尾的导航也会进 afterEach）与
 * router.onError（chunk 加载等异常不经过 afterEach）统一清理。
 * 连续导航（旧导航未完成又发起新的）由 beforeEach 重置计时覆盖，
 * 新导航取代旧反馈。视觉由 app.vue 挂载的 NavLoadingOverlay 渲染。
 */
import { defineNuxtPlugin, useRouter } from '#imports'

export default defineNuxtPlugin(() => {
  const { begin, end } = useNavLoading()
  const router = useRouter()

  router.beforeEach(() => {
    begin()
  })
  router.afterEach((_to, _from, _failure) => {
    // 成功与失败（含 NavigationFailure）都要卸载反馈：导航已结束
    end()
  })
  router.onError(() => {
    end()
  })
})
