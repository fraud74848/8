// frontend/api/index.ts
import request from "./request";
import type { Device, DeviceListResponse, User } from "./types";
import { fetchAllDevicesCached } from "./deviceListCache";

export const metaApi = {
  modules: () =>
    request.get<{ domainMonitor: boolean; callback: boolean; wsTicketRequired?: boolean }>(
      "/meta/modules",
    ),
};

export const deviceApi = {
  /** 分页拉取全部设备（后端单页最大 100 条）；30s 内存缓存 + 并发 dedup，force:true 强制刷新 */
  listAll: async (
    params?: Omit<
      {
        page?: number;
        pageSize?: number;
        status?: string;
        employeeSearch?: string;
        deviceStatus?: string;
        keyword?: string;
        force?: boolean;
      },
      "page" | "pageSize"
    >,
  ) => {
    const { force, ...query } = params ?? {};

    const fetchAll = async (listParams?: typeof query) => {
      const all: Device[] = [];
      let page = 1;
      const pageSize = 100;
      let total = 0;
      while (true) {
        const { data } = await request.get<DeviceListResponse>("/devices", {
          params: { ...listParams, page, pageSize },
        });
        all.push(...(data.items || []));
        total = data.total ?? all.length;
        if (all.length >= total || !(data.items?.length)) break;
        page++;
      }
      return {
        data: {
          items: all,
          total: all.length,
          page: 1,
          pageSize: all.length,
        } satisfies DeviceListResponse,
      };
    };

    return fetchAllDevicesCached(fetchAll, query, { force: force === true });
  },
};

export const userApi = {
  getProfile: () => request.get<User>("/user/profile"),
};
