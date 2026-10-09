/**
 * Same-origin proxy to mission-control. The operator API key lives only in
 * server runtime config and is attached here, so it never reaches the browser.
 * Put the dashboard behind SSO (e.g. ALB OIDC) to gate who can reach this.
 */
export default defineEventHandler((event) => {
  const { controlUrl, operatorKey } = useRuntimeConfig(event);
  const path = getRouterParam(event, 'path') ?? '';
  if (!/^(robots|commands|audit)(\/|$)/.test(path)) {
    throw createError({ statusCode: 404, statusMessage: 'unknown control route' });
  }
  const query = getRequestURL(event).search;
  return proxyRequest(event, `${controlUrl}/v1/${path}${query}`, {
    headers: { Authorization: `Bearer ${operatorKey}` },
  });
});
