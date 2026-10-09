import {
  Body,
  Controller,
  Get,
  HttpCode,
  Param,
  ParseIntPipe,
  Post,
  Query,
  Res,
  DefaultValuePipe,
  ParseEnumPipe,
} from '@nestjs/common';
import type { Response } from 'express';
import { CurrentPrincipal, Principal, Roles } from '../auth/auth';
import { CommandsService } from './commands.service';
import { AckDto, IssueCommandDto } from './dto';
import { CommandStatus } from './command.types';

const STATUSES = ['queued', 'sent', 'completed', 'rejected', 'cancelled', 'expired'] as const;
const statusEnum = Object.fromEntries(STATUSES.map((s) => [s, s]));

@Controller('v1')
export class CommandsController {
  constructor(private readonly commands: CommandsService) {}

  // ---- operator-facing ----

  @Post('robots/:robotId/commands')
  @Roles('operator')
  issue(@Param('robotId') robotId: string, @Body() dto: IssueCommandDto, @CurrentPrincipal() who: Principal) {
    return this.commands.issue(robotId, dto.type, dto.params, who.name);
  }

  @Get('robots')
  @Roles('operator')
  links() {
    return this.commands.links();
  }

  @Get('commands')
  @Roles('operator')
  list(
    @Query('robot_id') robotId?: string,
    @Query('status', new ParseEnumPipe(statusEnum, { optional: true })) status?: CommandStatus,
    @Query('limit', new DefaultValuePipe(100), ParseIntPipe) limit?: number,
  ) {
    return this.commands.list({ robotId, status, limit: Math.min(Math.max(limit ?? 100, 1), 500) });
  }

  @Get('commands/:id')
  @Roles('operator')
  get(@Param('id') id: string) {
    return this.commands.get(id);
  }

  @Post('commands/:id/cancel')
  @Roles('operator')
  @HttpCode(200)
  cancel(@Param('id') id: string, @CurrentPrincipal() who: Principal) {
    return this.commands.cancel(id, who.name);
  }

  @Get('audit')
  @Roles('operator')
  audit(@Query('limit', new DefaultValuePipe(200), ParseIntPipe) limit: number) {
    return this.commands.auditLog(Math.min(Math.max(limit, 1), 1000));
  }

  // ---- robot-facing ----

  /** POST, not GET: taking a command changes its state. 204 when idle. */
  @Post('robots/:robotId/commands/next')
  @Roles('robot')
  next(@Param('robotId') robotId: string, @Res({ passthrough: true }) res: Response) {
    const cmd = this.commands.next(robotId);
    if (!cmd) {
      res.status(204);
      return;
    }
    res.status(200);
    return cmd;
  }

  @Post('commands/:id/ack')
  @Roles('robot')
  @HttpCode(200)
  ack(@Param('id') id: string, @Body() dto: AckDto) {
    return this.commands.ack(id, dto.status, dto.reason);
  }
}
