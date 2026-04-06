import { apiGet } from "./client";

export interface NewsItem {
  id: string;
  title: string;
  link: string;
  source: string;
  published_at: string;
}

export interface NewsResponse {
  data: NewsItem[];
}

export async function getNews(limit = 50): Promise<NewsItem[]> {
  const response = await apiGet<NewsResponse>(`/news?limit=${limit}`);
  return response.data;
}
