// frontend/api/index.ts
import request from "./request";
// Current login identity returned by /auth/me.
export interface User {
  id: string;
  username: string;
  role: string;
  permissions: string[];
}

export const metaApi = {
  modules: () =>
    request.get<{ domainMonitor: boolean; callback: boolean; wsTicketRequired?: boolean }>(
      "/meta/modules",
    ),
};

export const userApi = {
  getProfile: () => request.get<User>("/auth/me"),
};
