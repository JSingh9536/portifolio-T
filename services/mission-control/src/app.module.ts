import { Module, ValidationPipe } from '@nestjs/common';
import { APP_GUARD, APP_PIPE } from '@nestjs/core';
import { AuthGuard, KeyStore } from './auth/auth';
import { Clock } from './clock';
import { CommandsController } from './commands/commands.controller';
import { CommandsService } from './commands/commands.service';
import { HealthController } from './health.controller';

@Module({
  controllers: [CommandsController, HealthController],
  providers: [
    Clock,
    CommandsService,
    { provide: KeyStore, useFactory: () => new KeyStore() },
    { provide: APP_GUARD, useClass: AuthGuard },
    {
      provide: APP_PIPE,
      useValue: new ValidationPipe({ whitelist: true, forbidNonWhitelisted: true, transform: true }),
    },
  ],
})
export class AppModule {}
