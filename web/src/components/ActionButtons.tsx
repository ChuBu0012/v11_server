import React, { useEffect, useState } from 'react';
import { RefreshCw, Zap, Check, AlertCircle, LoaderCircle } from 'lucide-react';
import { sendAction, subscribeAction } from '../lib/firebase';
import { ActionRequest } from '../lib/types';

export const ActionButtons: React.FC = () => {
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<{ text: string; isError?: boolean } | null>(null);
  const [cooldown, setCooldown] = useState(false);
  const [trackedAction, setTrackedAction] = useState<ActionRequest | null>(null);

  useEffect(() => {
    if (!trackedAction?.id) return;
    if (trackedAction.status === 'completed' || trackedAction.status === 'failed') {
      setLoadingAction(null);
    }
  }, [trackedAction]);

  const handleTrigger = async (action: string, label: string) => {
    if (cooldown || loadingAction) return;

    setLoadingAction(action);
    setStatusMessage(null);

    try {
      const res = await sendAction(action);
      setTrackedAction(null);
      setStatusMessage({ text: `ส่ง "${label}" แล้ว · กำลังรอ v11 ประมวลผล` });
      subscribeAction(res, (next) => setTrackedAction(next));
      setCooldown(true);
      setTimeout(() => setCooldown(false), 8000);
    } catch (err: any) {
      setStatusMessage({ text: `เกิดข้อผิดพลาด: ${err?.message || 'ส่งคำสั่งไม่สำเร็จ'}`, isError: true });
    } finally {
      setLoadingAction(null);
    }
  };

  return (
    <div className="action-panel">
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

      {(statusMessage || trackedAction) && (
        <div
          className={`mt-3 p-3 rounded-xl text-xs flex items-center space-x-2 animate-fadeIn ${
            statusMessage?.isError || trackedAction?.status === 'failed'
              ? 'bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-300 border border-red-200 dark:border-red-900'
              : trackedAction?.status === 'completed'
                ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                : 'bg-blue-50 dark:bg-blue-950/30 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-900'
          }`}
        >
          {statusMessage?.isError || trackedAction?.status === 'failed' ? (
            <AlertCircle className="w-4 h-4 shrink-0" />
          ) : trackedAction?.status === 'completed' ? (
            <Check className="w-4 h-4 shrink-0" />
          ) : (
            <LoaderCircle className="w-4 h-4 shrink-0 animate-spin" />
          )}
          <span>
            {trackedAction?.status === 'failed'
              ? trackedAction.result || 'v11 ประมวลคำสั่งไม่สำเร็จ'
              : trackedAction?.status === 'completed'
                ? `เสร็จสมบูรณ์ · ${trackedAction.result || 'ไม่มีรายละเอียดเพิ่มเติม'}`
                : trackedAction?.status === 'processing'
                  ? 'v11 กำลังประมวลคำสั่ง…'
                  : statusMessage?.text}
          </span>
        </div>
      )}
    </div>
  );
};
