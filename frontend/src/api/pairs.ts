import { apiGet } from "./client";
import type { ListResponse, Pair } from "../types/api";

export function fetchPairs() {
  return apiGet<ListResponse<Pair>>("/pairs");
}
