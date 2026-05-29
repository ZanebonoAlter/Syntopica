import type { FeedResponse } from '~/types'

export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  const { url } = body

  if (!url) {
    throw createError({
      statusCode: 400,
      statusMessage: 'URL is required'
    })
  }

  try {
    // Using RSS2JSON API as a CORS proxy
    const apiUrl = `https://api.rss2json.com/v1/api.json?rss_url=${encodeURIComponent(url)}`

    const response = await fetch(apiUrl)
    const payload = await response.json() as FeedResponse

    if (!response.ok || payload.status !== 'ok') {
      throw createError({
        statusCode: 400,
        statusMessage: 'Failed to parse RSS feed'
      })
    }

    return payload
  } catch (e) {
    throw createError({
      statusCode: 500,
      statusMessage: e instanceof Error ? e.message : 'Failed to fetch feed'
    })
  }
})
