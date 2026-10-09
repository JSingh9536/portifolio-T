import { Injectable } from '@nestjs/common';

/** Injectable time source so expiry logic is testable without sleeps. */
@Injectable()
export class Clock {
  now(): Date {
    return new Date();
  }
}
