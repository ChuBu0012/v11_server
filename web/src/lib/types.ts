export interface DeviceStatus {
  battery_level: number;
  is_charging: boolean;
  ram_used_percent: number;
  disk_used_percent: number;
  uptime_seconds: number;
  uptime_hours: number;
}

export interface ServicesStatus {
  server: string;
  gatus: string;
  tailscale: string;
}

export interface PublicStatus {
  updated_at: string;
  device: DeviceStatus;
  services: ServicesStatus;
  message?: string;
}

export interface GoldData {
  world_spot: number;
  bar_buy: number;
  bar_sell: number;
  updated_at?: string;
}

export interface StockItem {
  title: string;
  source?: string;
  url?: string;
}

export interface NewsItem {
  category: string;
  summary: string;
}

export interface PublicDigest {
  updated_at: string;
  gold?: GoldData;
  stocks?: StockItem[];
  news?: NewsItem[];
  summary_md?: string;
}

export interface ActionRequest {
  id?: string;
  action: string;
  status: 'pending' | 'processing' | 'completed' | 'failed';
  requested_at: string;
  completed_at?: string | null;
  result?: string | null;
}
