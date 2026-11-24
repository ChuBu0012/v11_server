import React from 'react';
import { Battery, BatteryCharging, Cpu, HardDrive, Clock, CheckCircle2, Server } from 'lucide-react';
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

  return (
    <div className="status-panel">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center space-x-2">
          <Server className="w-4 h-4 text-[#0066cc]" />
          <h2 className="text-sm font-semibold text-gray-900 dark:text-white uppercase tracking-wider">
            สุขภาพอุปกรณ์ & ระบบ
          </h2>
        </div>
        <span className="text-[11px] text-gray-400 dark:text-gray-500">
          อัปเดต: {timeAgo(updated_at)}
        </span>
      </div>

      <div className="metric-grid">
        {/* Battery */}
        <div className="bg-gray-50/80 dark:bg-gray-800/40 rounded-2xl p-3.5 border border-gray-100/80 dark:border-gray-800/60">
          <div className="flex items-center justify-between text-gray-500 dark:text-gray-400 mb-1.5">
            <span className="text-xs font-medium">แบตเตอรี่</span>
            {device.is_charging ? (
              <BatteryCharging className="w-4 h-4 text-emerald-500 animate-pulse" />
            ) : (
              <Battery className="w-4 h-4 text-blue-500" />
            )}
          </div>
          <div className="text-xl font-bold text-gray-900 dark:text-white">
            {device.battery_level}%
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 h-1.5 rounded-full mt-2 overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                device.battery_level < 20 ? 'bg-red-500' : device.battery_level < 50 ? 'bg-amber-500' : 'bg-emerald-500'
              }`}
              style={{ width: `${Math.min(100, Math.max(0, device.battery_level))}%` }}
            />
          </div>
        </div>

        {/* RAM */}
        <div className="bg-gray-50/80 dark:bg-gray-800/40 rounded-2xl p-3.5 border border-gray-100/80 dark:border-gray-800/60">
          <div className="flex items-center justify-between text-gray-500 dark:text-gray-400 mb-1.5">
            <span className="text-xs font-medium">หน่วยความจำ RAM</span>
            <Cpu className="w-4 h-4 text-purple-500" />
          </div>
          <div className="text-xl font-bold text-gray-900 dark:text-white">
            {device.ram_used_percent}%
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 h-1.5 rounded-full mt-2 overflow-hidden">
            <div
              className="h-full rounded-full bg-purple-500 transition-all duration-500"
              style={{ width: `${Math.min(100, Math.max(0, device.ram_used_percent))}%` }}
            />
          </div>
        </div>

        {/* Disk */}
        <div className="bg-gray-50/80 dark:bg-gray-800/40 rounded-2xl p-3.5 border border-gray-100/80 dark:border-gray-800/60">
          <div className="flex items-center justify-between text-gray-500 dark:text-gray-400 mb-1.5">
            <span className="text-xs font-medium">พื้นที่ดิสก์</span>
            <HardDrive className="w-4 h-4 text-sky-500" />
          </div>
          <div className="text-xl font-bold text-gray-900 dark:text-white">
            {device.disk_used_percent}%
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 h-1.5 rounded-full mt-2 overflow-hidden">
            <div
              className="h-full rounded-full bg-sky-500 transition-all duration-500"
              style={{ width: `${Math.min(100, Math.max(0, device.disk_used_percent))}%` }}
            />
          </div>
        </div>

        {/* Uptime */}
        <div className="bg-gray-50/80 dark:bg-gray-800/40 rounded-2xl p-3.5 border border-gray-100/80 dark:border-gray-800/60">
          <div className="flex items-center justify-between text-gray-500 dark:text-gray-400 mb-1.5">
            <span className="text-xs font-medium">เปิดต่อเนื่อง</span>
            <Clock className="w-4 h-4 text-amber-500" />
          </div>
          <div className="text-sm font-bold text-gray-900 dark:text-white truncate">
            {formatUptime(device.uptime_seconds)}
          </div>
          <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-2">
            ({device.uptime_hours} ชั่วโมง)
          </p>
        </div>
      </div>

      {/* Services status bar */}
      <div className="pt-3 border-t border-gray-100 dark:border-gray-800/60 flex flex-wrap items-center justify-between gap-2 text-xs">
        <div className="text-gray-500 dark:text-gray-400 font-medium">สถานะบริการหลัก:</div>
        <div className="flex items-center space-x-3">
          <span className="flex items-center space-x-1 text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="w-3.5 h-3.5" />
            <span>Go Server ({services.server})</span>
          </span>
          <span className="flex items-center space-x-1 text-emerald-600 dark:text-emerald-400">
            <CheckCircle2 className="w-3.5 h-3.5" />
            <span>Gatus ({services.gatus})</span>
          </span>
          <span className="flex items-center space-x-1 text-blue-600 dark:text-blue-400">
            <CheckCircle2 className="w-3.5 h-3.5" />
            <span>Mesh ({services.tailscale})</span>
          </span>
        </div>
      </div>
    </div>
  );
};
