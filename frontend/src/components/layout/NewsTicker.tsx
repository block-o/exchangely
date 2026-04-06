import React, { useEffect, useState, useMemo } from 'react';
import { NewsItem, getNews } from '../../api/news';
import { formatUnix } from '../../lib/format';
import { API_BASE_URL } from '../../api/client';

interface NewsTickerProps {
  assets?: string[]; // Symbols to highlight
}

export const NewsTicker: React.FC<NewsTickerProps> = ({ assets = [] }) => {
  const [news, setNews] = useState<NewsItem[]>([]);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    let active = true;

    // Initial fetch
    getNews(20).then(items => {
      if (active) setNews(items);
    });

    // SSE Stream
    const es = new EventSource(`${API_BASE_URL}/news/stream`);
    
    es.onopen = () => {
      if (active) setConnected(true);
    };

    es.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.news && active) {
          setNews(payload.news);
        }
      } catch (err) {
        console.error('Failed to parse news stream', err);
      }
    };

    es.onerror = () => {
      if (active) setConnected(false);
    };

    return () => {
      active = false;
      es.close();
    };
  }, []);

  const highlightText = (text: string, keywords: string[]) => {
    if (!keywords.length) return text;
    
    const regex = new RegExp(`\\b(${keywords.join('|')})\\b`, 'gi');
    const parts = text.split(regex);
    
    return parts.map((part, i) => (
      regex.test(part) ? (
        <span key={i} className="news-highlight">{part}</span>
      ) : part
    ));
  };

  if (news.length === 0) return null;

  // Duplicate news to ensure smooth infinite loop if news count is low
  const displayNews = [...news, ...news];

  return (
    <div className="news-ticker-container" title={connected ? "Connected to news stream" : "News stream disconnected"}>
      <div className="news-ticker-label">
        Latest News
      </div>
      <div className="news-ticker-scroll">
        {displayNews.map((item, idx) => (
          <a 
            key={`${item.id}-${idx}`} 
            href={item.link} 
            target="_blank" 
            rel="noopener noreferrer" 
            className="news-item"
          >
            <span className="news-item-source">{item.source}</span>
            <span className="news-item-title">
              {highlightText(item.title, assets)}
            </span>
            <span className="news-item-time">
              {formatUnix(Math.floor(new Date(item.published_at).getTime() / 1000))}
            </span>
          </a>
        ))}
      </div>
    </div>
  );
};
