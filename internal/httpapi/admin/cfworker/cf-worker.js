// ds2api Cloudflare Worker — reverse proxy for chat.deepseek.com
//
// Deployment:
//   1. npx wrangler deploy (or copy-paste to CF Dashboard → Workers)
//   2. Set environment variable TARGET_HOST = chat.deepseek.com
//   3. Update ds2api config: replace chat.deepseek.com with your worker domain
//
// Why this helps:
//   - Each CF edge location has different egress IPs
//   - Requests appear to come from diverse IPs instead of your single server
//   - Free tier: 100k requests/day per worker
//
// Caveats:
//   - CF IPs may be flagged by DeepSeek as datacenter traffic
//   - Extra ~5-50ms latency from CF edge hop
//   - Not a replacement for residential proxies, but additive diversity

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url)
    const targetHost = env.TARGET_HOST || 'chat.deepseek.com'

    // Build target URL: preserve path + query
    const targetURL = new URL(url.pathname + url.search, `https://${targetHost}`)

    // Forward request with same method, headers, and body
    const headers = new Headers(request.headers)
    headers.set('Host', targetHost)

    // Remove CF-specific headers that might leak the proxy
    headers.delete('CF-Connecting-IP')
    headers.delete('CF-IPCountry')
    headers.delete('CF-Ray')
    headers.delete('CF-Visitor')
    headers.delete('X-Forwarded-For')
    headers.delete('X-Forwarded-Proto')
    headers.delete('X-Real-IP')

    const proxyRequest = new Request(targetURL, {
      method: request.method,
      headers: headers,
      body: request.body,
      redirect: 'follow',
    })

    let response = await fetch(proxyRequest)

    // For SSE streams, ensure chunked transfer works
    // CF Workers automatically handle streaming responses

    // Build clean response headers
    const responseHeaders = new Headers(response.headers)
    responseHeaders.set('Access-Control-Allow-Origin', '*')
    responseHeaders.set('X-Proxied-By', 'ds2api-cf-worker')

    return new Response(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    })
  },
}
