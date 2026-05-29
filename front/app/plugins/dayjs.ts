import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'

dayjs.extend(utc)
dayjs.extend(timezone)
dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

declare module '#app' {
  interface NuxtApp {
    $dayjs: typeof dayjs
  }
}

declare module 'vue' {
  interface ComponentCustomProperties {
    $dayjs: typeof dayjs
  }
}

export default defineNuxtPlugin(() => {
  return {
    provide: {
      dayjs
    }
  }
})
