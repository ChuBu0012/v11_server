import React, { useState, useEffect } from 'react';
import { Sparkles, Send, Copy, Check, AlertCircle, LoaderCircle, RotateCcw } from 'lucide-react';
import { sendAction, subscribeAction } from '../lib/firebase';
import { ActionRequest } from '../lib/types';

export const AskCard: React.FC = () => {
  const [prompt, setPrompt] = useState('');
  const [model, setModel] = useState<'auto' | 'groq' | 'gemini'>('auto');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [actionId, setActionId] = useState<string | null>(null);
  const [trackedAction, setTrackedAction] = useState<ActionRequest | null>(null);

  useEffect(() => {
    if (!actionId) return;
    return subscribeAction(actionId, (next) => {
      setTrackedAction(next);
      if (next?.status === 'completed' || next?.status === 'failed') {
        setLoading(false);
        if (next?.status === 'failed') {
          setError(next.result || 'คำขอไม่สำเร็จ');
        }
      }
    });
  }, [actionId]);

  const handleSubmit = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    const q = prompt.trim();
    if (!q || loading) return;

    setLoading(true);
    setError(null);
    setCopied(false);
    setTrackedAction(null);

    try {
      const id = await sendAction('ask', { prompt: q, model });
      setActionId(id);
    } catch (err: any) {
      setLoading(false);
      setError(err?.message || 'ส่งคำถามไม่สำเร็จ');
    }
  };

  const handleCopy = async () => {
    if (!trackedAction?.result) return;
    try {
      await navigator.clipboard.writeText(trackedAction.result);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (_) {}
  };

  const handleClear = () => {
    setPrompt('');
    setActionId(null);
    setTrackedAction(null);
    setError(null);
  };

  return (
    <section className="operation-card" aria-labelledby="ask-title" style={{ marginTop: '16px' }}>
      <div className="card-top-rail">
        <div>
          <span className="section-kicker">03 / AI QUERY</span>
          <h2 id="ask-title" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Sparkles style={{ width: '18px', height: '18px', color: 'var(--accent, #3b82f6)' }} />
            <span>Ask AI</span>
          </h2>
        </div>
        <div style={{ display: 'flex', gap: '4px', background: 'var(--surface-subtle, rgba(255,255,255,0.05))', padding: '2px', borderRadius: '6px' }}>
          {(['auto', 'groq', 'gemini'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setModel(m)}
              style={{
                border: 'none',
                background: model === m ? 'var(--surface-active, rgba(255,255,255,0.15))' : 'transparent',
                color: model === m ? 'var(--text-primary, #fff)' : 'var(--text-muted, #888)',
                padding: '4px 10px',
                borderRadius: '4px',
                fontSize: '12px',
                fontWeight: model === m ? 600 : 400,
                cursor: 'pointer',
                textTransform: 'uppercase',
                transition: 'all 0.15s ease'
              }}
            >
              {m}
            </button>
          ))}
        </div>
      </div>

      <form onSubmit={handleSubmit} style={{ marginTop: '14px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
        <div style={{ display: 'flex', gap: '8px' }}>
          <input
            type="text"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="ถามคำถาม AI... (เช่น สรุปข่าวทองวันนี้, สุขภาพเครื่อง)"
            disabled={loading}
            style={{
              flex: 1,
              padding: '10px 14px',
              borderRadius: '8px',
              border: '1px solid var(--border-subtle, rgba(255,255,255,0.1))',
              background: 'var(--surface-input, rgba(255,255,255,0.03))',
              color: 'var(--text-primary, inherit)',
              fontSize: '14px',
              outline: 'none'
            }}
          />
          <button
            type="submit"
            disabled={loading || !prompt.trim()}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              padding: '0 16px',
              borderRadius: '8px',
              border: 'none',
              background: 'var(--btn-primary, #3b82f6)',
              color: '#ffffff',
              fontWeight: 600,
              fontSize: '14px',
              cursor: loading || !prompt.trim() ? 'not-allowed' : 'pointer',
              opacity: loading || !prompt.trim() ? 0.6 : 1,
              transition: 'all 0.15s ease'
            }}
          >
            {loading ? <LoaderCircle className="spin" style={{ width: '16px', height: '16px' }} /> : <Send style={{ width: '16px', height: '16px' }} />}
            <span>{loading ? 'กำลังคิด…' : 'ถาม'}</span>
          </button>
        </div>

        <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', alignItems: 'center', fontSize: '12px' }}>
          <span style={{ color: 'var(--text-muted, #888)' }}>แนะนำ:</span>
          {[
            'ทำไมทองคำราคาถึงขึ้นลง',
            'สรุปสถานะสุขภาพเครื่องตอนนี้',
            'อธิบายควอนตัมคอมพิวติ้งแบบเข้าใจง่าย'
          ].map((suggestion) => (
            <button
              key={suggestion}
              type="button"
              onClick={() => {
                setPrompt(suggestion);
              }}
              disabled={loading}
              style={{
                border: '1px solid var(--border-subtle, rgba(255,255,255,0.1))',
                background: 'transparent',
                color: 'var(--text-secondary, #aaa)',
                padding: '2px 8px',
                borderRadius: '12px',
                fontSize: '11.5px',
                cursor: 'pointer'
              }}
            >
              {suggestion}
            </button>
          ))}
        </div>
      </form>

      {error && (
        <div style={{ marginTop: '12px', display: 'flex', alignItems: 'center', gap: '8px', color: '#ef4444', fontSize: '13px' }}>
          <AlertCircle style={{ width: '16px', height: '16px' }} />
          <span>{error}</span>
        </div>
      )}

      {trackedAction && trackedAction.status === 'completed' && trackedAction.result && (
        <div style={{ marginTop: '16px', paddingTop: '14px', borderTop: '1px solid var(--border-subtle, rgba(255,255,255,0.08))' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
            <span style={{ fontSize: '12px', fontWeight: 600, color: '#10b981', display: 'flex', alignItems: 'center', gap: '4px' }}>
              ✓ คำตอบจาก AI ({model.toUpperCase()})
            </span>
            <div style={{ display: 'flex', gap: '6px' }}>
              <button
                type="button"
                onClick={handleCopy}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  border: '1px solid var(--border-subtle, rgba(255,255,255,0.1))',
                  background: 'transparent',
                  color: 'var(--text-secondary, #aaa)',
                  padding: '3px 8px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  cursor: 'pointer'
                }}
              >
                {copied ? <Check style={{ width: '14px', height: '14px', color: '#10b981' }} /> : <Copy style={{ width: '14px', height: '14px' }} />}
                <span>{copied ? 'คัดลอกแล้ว' : 'คัดลอก'}</span>
              </button>
              <button
                type="button"
                onClick={handleClear}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  border: '1px solid var(--border-subtle, rgba(255,255,255,0.1))',
                  background: 'transparent',
                  color: 'var(--text-secondary, #aaa)',
                  padding: '3px 8px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  cursor: 'pointer'
                }}
              >
                <RotateCcw style={{ width: '14px', height: '14px' }} />
                <span>ล้าง</span>
              </button>
            </div>
          </div>
          <div
            style={{
              fontSize: '14px',
              lineHeight: 1.6,
              color: 'var(--text-primary, inherit)',
              whiteSpace: 'pre-wrap',
              background: 'var(--surface-input, rgba(255,255,255,0.02))',
              padding: '12px 14px',
              borderRadius: '8px',
              border: '1px solid var(--border-subtle, rgba(255,255,255,0.05))'
            }}
          >
            {trackedAction.result}
          </div>
        </div>
      )}
    </section>
  );
};
