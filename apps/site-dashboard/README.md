# site-dashboard (Nuxt 4 / Vue 3)

This is the live site console. It shows an interpolated surface-uplift map, per-robot telemetry with pressure sparklines, operator controls, and the command log.

- Telemetry comes straight from `telemetry-ingest`: a seed from `/latest`, then the SSE stream.
- Control calls go to the dashboard's own `/api/control/*` Nitro route, which adds the operator key on the server side.
- Pure logic lives in `app/utils` and is unit-tested with Vitest: the fleet-state fold, IDW interpolation, and the color ramp.

| Variable | Exposure | Default |
|---|---|---|
| `NUXT_PUBLIC_INGEST_URL` | browser | `http://localhost:8080` |
| `NUXT_PUBLIC_SITE_ID` | browser | `alameda-pilot` |
| `NUXT_CONTROL_URL` | server only | `http://localhost:3001` |
| `NUXT_OPERATOR_KEY` | server only | `dev-operator` |

```bash
npm ci && npm run dev        # http://localhost:3000
npm run typecheck && npm test && npm run build
```
