import 'reflect-metadata';
import { INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import request from 'supertest';
import { AppModule } from '../src/app.module';
import { KeyStore } from '../src/auth/auth';

const OP = { Authorization: 'Bearer op-key' };
const ROBOT = { Authorization: 'Bearer robot-key' };

describe('mission-control HTTP', () => {
  let app: INestApplication;

  beforeAll(async () => {
    const mod = await Test.createTestingModule({ imports: [AppModule] })
      .overrideProvider(KeyStore)
      .useValue(new KeyStore({ OPERATOR_KEYS: 'alice:op-key', ROBOT_KEY: 'robot-key' }))
      .compile();
    app = mod.createNestApplication();
    await app.init();
  });
  afterAll(() => app.close());

  it('health is public', () => request(app.getHttpServer()).get('/healthz').expect(200, { status: 'ok' }));

  it('requires auth and the right role', async () => {
    const http = request(app.getHttpServer());
    await http.post('/v1/robots/tn-01/commands').send({ type: 'pause' }).expect(401);
    await http.post('/v1/robots/tn-01/commands').set({ Authorization: 'Bearer nope' }).send({ type: 'pause' }).expect(401);
    await http.post('/v1/robots/tn-01/commands').set(ROBOT).send({ type: 'pause' }).expect(403);
    await http.post('/v1/robots/tn-01/commands/next').set(OP).expect(403);
  });

  it('validates bodies', async () => {
    const http = request(app.getHttpServer());
    await http.post('/v1/robots/tn-01/commands').set(OP).send({ type: 'self_destruct' }).expect(400);
    await http.post('/v1/robots/tn-01/commands').set(OP).send({ type: 'pause', sneaky: 1 }).expect(400);
    await http.post('/v1/robots/tn-01/commands').set(OP).send({ type: 'set_flow', params: { flow_lpm: 'lots' } }).expect(400);
    await http.get('/v1/commands?status=bogus').set(OP).expect(400);
  });

  it('round-trips a command from operator to robot and back', async () => {
    const http = request(app.getHttpServer());
    const issued = await http
      .post('/v1/robots/tn-07/commands')
      .set(OP)
      .send({ type: 'set_flow', params: { flow_lpm: 120 } })
      .expect(201);
    expect(issued.body).toMatchObject({ robot_id: 'tn-07', status: 'queued', issued_by: 'alice' });

    const next = await http.post('/v1/robots/tn-07/commands/next').set(ROBOT).expect(200);
    expect(next.body).toMatchObject({ id: issued.body.id, type: 'set_flow', params: { flow_lpm: 120 } });
    await http.post('/v1/robots/tn-07/commands/next').set(ROBOT).expect(204);

    await http.post(`/v1/commands/${issued.body.id}/ack`).set(ROBOT).send({ status: 'completed' }).expect(200);
    await http.post(`/v1/commands/${issued.body.id}/ack`).set(ROBOT).send({ status: 'completed' }).expect(409);

    const list = await http.get('/v1/commands?robot_id=tn-07&status=completed').set(OP).expect(200);
    expect(list.body).toHaveLength(1);
    const audit = await http.get('/v1/audit').set(OP).expect(200);
    expect(audit.body[0]).toMatchObject({ command_id: issued.body.id, to: 'completed' });
  });

  it('404s unknown commands', () =>
    request(app.getHttpServer()).get('/v1/commands/does-not-exist').set(OP).expect(404));
});
