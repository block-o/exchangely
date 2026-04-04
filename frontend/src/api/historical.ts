import { apiGet } from "./client";
import type { Candle, ListResponse } from "../types/api";

export function fetchHistorical(pair: string, interval = "1h", startTime?: number, endTime?: number) {
  const params = new URLSearchParams({ interval });
  if (typeof startTime === "number") {
    params.set("start_time", String(startTime));
  }
  if (typeof endTime === "number") {
    params.set("end_time", String(endTime));
  }
  return apiGet<ListResponse<Candle>>(`/historical/${pair}?${params.toString()}`);
}
