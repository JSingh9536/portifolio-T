# mission-control (NestJS)

This is the operator command-and-control API. Operators issue commands, robots pull and acknowledge them, and every state change is audited.

| Route | Role | Purpose |
|---|---|---|
| `POST /v1/robots/:id/commands` | operator | `{type: pause\|resume\|estop\|clear_fault\|set_flow, params?}` |
| `GET /v1/commands?robot_id&status&limit` | operator | Recent commands |
| `POST /v1/commands/:id/cancel` | operator | Cancel a queued command |
| `GET /v1/robots` | operator | Control-link status (last poll time, pending count) |
| `GET /v1/audit` | operator | Every transition, with the actor who caused it |
| `POST /v1/robots/:id/commands/next` | robot | Take the next command (`204` if none). An e-stop is always delivered first |
| `POST /v1/commands/:id/ack` | robot | `{status: completed\|rejected, reason?}` |

Auth uses `Authorization: Bearer <key>`. Keys are set with `OPERATOR_KEYS="alice:key1,bob:key2"` and `ROBOT_KEY=...`; without them the service falls back to dev keys and logs a warning.

Timing limits (see `src/commands/command.types.ts`):
- Queued commands expire after 120 s. E-stops never expire.
- A command that was sent but not acknowledged expires after 30 s.

```bash
npm ci && npm test && npm run build && npm start   # :3001
```
