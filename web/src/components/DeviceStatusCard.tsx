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
    if (days > 0) return `${days}d ${hours}h`;
    return `${hours}h ${mins}m`;
  };

  const timeAgo = (dateStr: string) => {
    const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
    if (diff < 10) return 'just now';
    if (diff < 60) return `${diff}s ago`;
    return `${Math.floor(diff / 60)}m ago`;
  };

  const metricTone = (value: number) => value >= 85 ? 'danger' : value >= 70 ? 'warning' : 'normal';

  const metrics = [
    { label: 'Battery', value: `${device.battery_level}%`, detail: device.is_charging ? 'Charging' : 'On battery', progress: device.battery_level, icon: device.is_charging ? BatteryCharging : Battery, tone: device.battery_level < 20 ? 'danger' : 'normal' },
    { label: 'Memory', value: `${device.ram_used_percent}%`, detail: 'Host RAM', progress: device.ram_used_percent, icon: Cpu, tone: metricTone(device.ram_used_percent) },
    { label: 'Storage', value: `${device.disk_used_percent}%`, detail: 'Internal', progress: device.disk_used_percent, icon: HardDrive, tone: metricTone(device.disk_used_percent) },
    { label: 'Uptime', value: formatUptime(device.uptime_seconds), detail: `${device.uptime_hours} hours`, progress: 100, icon: Clock, tone: 'normal' },
  ];

  const serviceList = [
    { name: 'Go server', state: services.server },
    { name: 'Gatus', state: services.gatus },
    { name: 'Tailscale', state: services.tailscale },
  ];

  return (
    <section className="status-panel" aria-labelledby="status-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">01 / DEVICE</span>
          <h2 id="status-title">Current health</h2>
        </div>
        <span className="updated-label">Updated {timeAgo(updated_at)}</span>
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
        {serviceList.map((service) => (
          <div className="service-item" key={service.name}>
            {service.state === 'healthy' || service.state === 'connected'
              ? <CheckCircle2 />
              : <CircleAlert />}
            <span>{service.name}</span>
            <strong>{service.state}</strong>
          </div>
        ))}
      </div>

      <div className="server-identity">
        <Server />
        <span>Vivo 1804</span>
        <strong>ARM64 / Termux</strong>
      </div>
    </section>
  );
};
