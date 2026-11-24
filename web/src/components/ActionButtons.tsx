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
      setStatusMessage({ text: `Queued · ${label}` });
      setCooldown(true);
      window.setTimeout(() => setCooldown(false), 8000);
    } catch (err: any) {
      setLoadingAction(null);
      setStatusMessage({ text: err?.message || 'Command unavailable', isError: true });
    }
  };

  const stateText = trackedAction?.status === 'failed'
    ? trackedAction.result || 'v11 rejected the command'
    : trackedAction?.status === 'completed'
      ? `Completed · ${trackedAction.result || 'No additional details'}`
      : trackedAction?.status === 'processing'
        ? 'v11 is processing…'
        : statusMessage?.text;

  return (
    <section className="action-panel" aria-labelledby="actions-title">
      <div className="panel-heading command-heading">
        <div>
          <span className="section-kicker">02 / CONTROL</span>
          <h2 id="actions-title">Quick actions</h2>
        </div>
        <span className="safe-label">SAFE</span>
      </div>

      <div className="command-list">
        <button
          className="command-button primary"
          onClick={() => handleTrigger('refresh_digest', 'Refresh digest')}
          disabled={loadingAction !== null || cooldown}
        >
          <span className="command-icon"><RefreshCw className={loadingAction === 'refresh_digest' ? 'spin' : ''} /></span>
          <span><strong>Refresh digest</strong><small>Generate a fresh daily summary</small></span>
          <span className="command-arrow">→</span>
        </button>

        <button
          className="command-button"
          onClick={() => handleTrigger('ping', 'Ping v11')}
          disabled={loadingAction !== null || cooldown}
        >
          <span className="command-icon"><Zap /></span>
          <span><strong>Ping v11</strong><small>Check device response</small></span>
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
