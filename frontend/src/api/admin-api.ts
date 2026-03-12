import { paperlessApi } from './client';

export async function listUsers(params?: { status?: string }): Promise<{ items: any[] }> {
  const query = params?.status ? `?noPaging=true&status=${params.status}` : '?noPaging=true';
  return paperlessApi.get(`/users${query}`);
}

export async function listRoles(): Promise<{ items: any[] }> {
  return paperlessApi.get('/roles?noPaging=true');
}
