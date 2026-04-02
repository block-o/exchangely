import { apiGet } from "./client";
import type { Asset, ListResponse } from "../types/api";

export function fetchAssets() {
  return apiGet<ListResponse<Asset>>("/assets");
}
