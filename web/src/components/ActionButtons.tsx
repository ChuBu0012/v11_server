import React, { useEffect, useState } from 'react';
import { RefreshCw, Zap, Check, AlertCircle, LoaderCircle } from 'lucide-react';
import { sendAction, subscribeAction } from '../lib/firebase';
import { ActionRequest } from '../lib/types';

export const ActionButtons: React.FC = () => {
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [statusMessage, setStatusMessage] = useState<{ text: string; isError?: boolean } | null>(null);
  const [cooldown, setCooldown] = useState(false);
  const [trackedAction, setTrackedAction] = useState<ActionRequest | null>(null);
  const [actionId, setActionId] = useState<string | null>(null);

  useEffect(() => {
    if (!actionId) return;
    return subscribeAction(actionId, (next) => {
      setTrackedAction(next);
      if (next?.status === 'completed' || next?.status === 'failed') setLoadingAction(null);
    });
  }, [actionId]);

  const handleTrigger = async (action: string, label: string) => {
    if (cooldown || loadingAction) return;
    setLoadingAction(action);
    setStatusMessage(null);
    setTrackedAction(null);

    try {
      const id = await sendAction(action);
      setActionId(id);
      setStatusMessage({ text: `ส่งคำสั่งแล้ว · ${label}` });
      setCooldown(true);
      window.setTimeout(() => setCooldown(false), 8000);
    } catch (err: any) {
      setLoadingAction(null);
      setStatusMessage({ text: err?.message || 'ไม่สามารถส่งคำสั่งได้', isError: true });
    }
  };

  const stateText = trackedAction?.status === 'failed'
    ? trackedAction.result || 'เครื่องปฏิเสธคำสั่ง'
    : trackedAction?.status === 'completed'
      ? `สำเร็จ · ${trackedAction.result || 'เสร็จสิ้น'}`
      : trackedAction?.status === 'processing'
        ? 'v11 กำลังประมวลผล…'
        : statusMessage?.text;

  return (
    <section className="action-panel" aria-labelledby="actions-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">คำสั่งด่วน</span>
          <h2 id="actions-title">ควบคุมจากระยะไกล</h2>
        </div>
        <span className="safe-label">SAFE</span>
      </div>

      <div className="command-list">
        <button
          className="command-button primary"
          onClick={() => handleTrigger('refresh_digest', 'อัปเดตสรุปข่าว')}
          disabled={loadingAction !== null || cooldown}
        >
          <span className="command-icon"><RefreshCw className={loadingAction === 'refresh_digest' ? 'spin' : ''} /></span>
          <span><strong>อัปเดตสรุปข่าว</strong><small>ดึงข่าวและราคาทองล่าสุด</small></span>
          <span className="command-arrow">→</span>
        </button>

        <button
          className="command-button"
          onClick={() => handleTrigger('ping', 'Ping v11')}
          disabled={loadingAction !== null || cooldown}
        >
          <span className="command-icon"><Zap /></span>
          <span><strong>เช็กการตอบสนอง (Ping)</strong><small>ทดสอบการเชื่อมต่อ v11</small></span>
          <span className="command-arrow">→</span>
        </button>
      </div>

      {(statusMessage || trackedAction) && (
        <div className={`command-status ${statusMessage?.isError || trackedAction?.status === 'failed' ? 'error' : trackedAction?.status === 'completed' ? 'success' : 'waiting'}`}>
          {statusMessage?.isError || trackedAction?.status === 'failed' ? <AlertCircle /> : trackedAction?.status === 'completed' ? <Check /> : <LoaderCircle className="spin" />}
          <span>{stateText}</span>
        </div>
      )}
    </section>
  );
};
