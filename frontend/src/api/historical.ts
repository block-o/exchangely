import { apiGet } from "./client";
import type { Candle, ListResponse } from "../types/api";

export function fetchHistorical(pair: string, interval = "1h") {
  return apiGet<ListResponse<Candle>>(`/historical/${pair}?interval=${interval}`);
}
