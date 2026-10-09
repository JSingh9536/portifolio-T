import {
  CanActivate,
  ExecutionContext,
  Injectable,
  Logger,
  SetMetadata,
  UnauthorizedException,
  ForbiddenException,
  createParamDecorator,
} from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { timingSafeEqual } from 'node:crypto';

export type Role = 'operator' | 'robot';
export interface Principal {
  role: Role;
  name: string;
}

const ROLES_KEY = 'roles';
/** Restrict a route to the given roles. Routes without @Roles are public. */
export const Roles = (...roles: Role[]) => SetMetadata(ROLES_KEY, roles);

export const CurrentPrincipal = createParamDecorator(
  (_: unknown, ctx: ExecutionContext): Principal => ctx.switchToHttp().getRequest().principal,
);

/**
 * Bearer-token auth. Operators get named keys so every command in the audit
 * log is attributable; robots share a fleet key (per-device mTLS is the
 * production answer).
 *
 *   OPERATOR_KEYS="alice:k1,bob:k2"   ROBOT_KEY="fleet-secret"
 */
export class KeyStore {
  private readonly keys: { key: Buffer; principal: Principal }[] = [];

  constructor(env: NodeJS.ProcessEnv = process.env) {
    const log = new Logger('Auth');
    const operators = env.OPERATOR_KEYS ?? 'dev:dev-operator';
    const robot = env.ROBOT_KEY ?? 'dev-robot';
    if (!env.OPERATOR_KEYS || !env.ROBOT_KEY) log.warn('Using development API keys; set OPERATOR_KEYS and ROBOT_KEY');
    for (const pair of operators.split(',').filter(Boolean)) {
      const [name, key] = pair.split(':');
      if (name && key) this.keys.push({ key: Buffer.from(key), principal: { role: 'operator', name } });
    }
    this.keys.push({ key: Buffer.from(robot), principal: { role: 'robot', name: 'fleet' } });
  }

  lookup(token: string): Principal | undefined {
    const t = Buffer.from(token);
    return this.keys.find(({ key }) => key.length === t.length && timingSafeEqual(key, t))?.principal;
  }
}

@Injectable()
export class AuthGuard implements CanActivate {
  constructor(
    private readonly reflector: Reflector,
    private readonly keys: KeyStore,
  ) {}

  canActivate(ctx: ExecutionContext): boolean {
    const roles = this.reflector.getAllAndOverride<Role[] | undefined>(ROLES_KEY, [ctx.getHandler(), ctx.getClass()]);
    if (!roles) return true;
    const req = ctx.switchToHttp().getRequest();
    const header: string = req.headers['authorization'] ?? '';
    const principal = header.startsWith('Bearer ') ? this.keys.lookup(header.slice(7)) : undefined;
    if (!principal) throw new UnauthorizedException('missing or invalid API key');
    if (!roles.includes(principal.role)) throw new ForbiddenException(`requires role: ${roles.join(' or ')}`);
    req.principal = principal;
    return true;
  }
}
