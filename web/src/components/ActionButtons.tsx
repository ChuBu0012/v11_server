import React, { useState } from 'react';
import { RefreshCw, Zap, Check, AlertCircle } from 'lucide-react';
import { sendAction } from '../lib/firebase';

export const ActionButtons: React.FC = () => {
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<{ text: string; isError?: boolean } | null>(null);
  const [cooldown, setCooldown] = useState(false);

  const handleTrigger = async (action: string, label: string) => {
    if (cooldown || loadingAction) return;

    setLoadingAction(action);
    setStatusMessage(null);

    try {
      const res = await sendAction(action);
      setStatusMessage({ text: `ส่งคำสั่ง "${label}" สำเร็จ (ID: ${res}) ระบบกำลังทำงานเบื้องหลัง` });
      setCooldown(true);
      setTimeout(() => setCooldown(false), 8000);
    } catch (err: any) {
      setStatusMessage({ text: `เกิดข้อผิดพลาด: ${err?.message || 'ส่งคำสั่งไม่สำเร็จ'}`, isError: true });
    } finally {
      setLoadingAction(null);
    }
  };

  return (
    <div className="bg-white dark:bg-[#1a1b23] border border-gray-100 dark:border-gray-800/80 rounded-[20px] p-5 shadow-sm">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-white uppercase tracking-wider">
          การสั่งงานพื้นฐาน (Safe Actions)
        </h2>
        <span className="text-[11px] text-gray-400">ควบคุมเฉพาะงานที่ปลอดภัย</span>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <button
          onClick={() => handleTrigger('refresh_digest', 'ดึงสรุปข่าวใหม่')}
          disabled={loadingAction !== null || cooldown}
          className="flex items-center justify-center space-x-2 py-3 px-4 rounded-2xl font-medium text-xs text-white bg-[#0066cc] hover:bg-[#0052a3] active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed shadow-sm"
        >
          <RefreshCw className={`w-4 h-4 ${loadingAction === 'refresh_digest' ? 'animate-spin' : ''}`} />
          <span>{loadingAction === 'refresh_digest' ? 'กำลังส่งคำสั่ง...' : '🔄 สั่งดึงสรุปข่าวใหม่ทันที'}</span>
        </button>

        <button
          onClick={() => handleTrigger('ping', 'Ping ตรวจระบบ')}
          disabled={loadingAction !== null || cooldown}
          className="flex items-center justify-center space-x-2 py-3 px-4 rounded-2xl font-medium text-xs text-gray-800 dark:text-gray-200 bg-gray-100 hover:bg-gray-200 dark:bg-gray-800 dark:hover:bg-gray-700 active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Zap className={`w-4 h-4 text-amber-500 ${loadingAction === 'ping' ? 'animate-bounce' : ''}`} />
          <span>{loadingAction === 'ping' ? 'กำลังส่ง Ping...' : '⚡ Ping ตรวจการตอบสนอง'}</span>
        </button>
      </div>

      {statusMessage && (
        <div
          className={`mt-3 p-3 rounded-xl text-xs flex items-center space-x-2 animate-fadeIn ${
            statusMessage.isError
              ? 'bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-300 border border-red-200 dark:border-red-900'
              : 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
          }`}
        >
          {statusMessage.isError ? (
            <AlertCircle className="w-4 h-4 flex-shrink-0" />
          ) : (
            <Check className="w-4 h-4 flex-shrink-0" />
          )}
          <span>{statusMessage.text}</span>
        </div>
      )}
    </div>
  );
};
