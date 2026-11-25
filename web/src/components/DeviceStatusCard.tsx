import React from 'react';
import { Battery, BatteryCharging, Cpu, HardDrive, Clock, CheckCircle2, CircleAlert, Server } from 'lucide-react';
import { PublicStatus } from '../lib/types';

interface DeviceStatusCardProps {
  status: PublicStatus;
}

export const DeviceStatusCard: React.FC<DeviceStatusCardProps> = ({ status }) => {
  const { device, services, updated_at } = status;

  const formatUptime = (seconds: number) => {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days} วัน ${hours} ชม.`;
    return `${hours} ชม. ${mins} นาที`;
  };

  const timeAgo = (dateStr: string) => {
    const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
    if (diff < 10) return 'เมื่อสักครู่';
    if (diff < 60) return `${diff} วินาทีที่แล้ว`;
    return `${Math.floor(diff / 60)} นาทีที่แล้ว`;
  };

  const metricTone = (value: number) => value >= 85 ? 'danger' : value >= 70 ? 'warning' : 'normal';

  const metrics = [
    { label: 'แบตเตอรี่', value: `${device.battery_level}%`, detail: device.is_charging ? 'กำลังชาร์จ' : 'ใช้งานแบต', progress: device.battery_level, icon: device.is_charging ? BatteryCharging : Battery, tone: device.battery_level < 20 ? 'danger' : 'normal' },
    { label: 'หน่วยความจำ RAM', value: `${device.ram_used_percent}%`, detail: 'การใช้งานเครื่อง', progress: device.ram_used_percent, icon: Cpu, tone: metricTone(device.ram_used_percent) },
    { label: 'พื้นที่เก็บข้อมูล', value: `${device.disk_used_percent}%`, detail: 'หน่วยความจำภายใน', progress: device.disk_used_percent, icon: HardDrive, tone: metricTone(device.disk_used_percent) },
    { label: 'เปิดต่อเนื่อง', value: formatUptime(device.uptime_seconds), detail: `${device.uptime_hours} ชั่วโมง`, progress: 100, icon: Clock, tone: 'normal' },
  ];

  const serviceList = [
    { name: 'Go Server', state: services.server },
    { name: 'Gatus', state: services.gatus },
    { name: 'Tailscale', state: services.tailscale },
  ];

  return (
    <section className="status-panel" aria-labelledby="status-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">01 · สถานะเครื่อง</span>
          <h2 id="status-title">สุขภาพระบบ vivo v11</h2>
        </div>
        <span className="updated-label">อัปเดต {timeAgo(updated_at)}</span>
      </div>

      <div className="metric-grid">
        {metrics.map((metric) => {
          const Icon = metric.icon;
          return (
            <article className={`metric-item ${metric.tone}`} key={metric.label}>
              <div className="metric-label-row">
                <span>{metric.label}</span>
                <Icon />
              </div>
              <strong>{metric.value}</strong>
              <small>{metric.detail}</small>
              <div className="metric-track">
                <span style={{ width: `${Math.min(100, Math.max(0, metric.progress))}%` }} />
              </div>
            </article>
          );
        })}
      </div>

      <div className="service-strip">
        {serviceList.map((service) => {
          const ok = service.state === 'healthy' || service.state === 'connected';
          return (
            <div className={`service-item ${ok ? 'ok' : 'down'}`} key={service.name}>
              {ok ? <CheckCircle2 /> : <CircleAlert />}
              <span>{service.name}</span>
              <strong>{ok ? 'ปกติ' : service.state}</strong>
            </div>
          );
        })}
      </div>

      <div className="server-identity">
        <Server />
        <span>vivo 1804 · Android Termux</span>
        <strong>ARM64</strong>
      </div>
    </section>
  );
};
