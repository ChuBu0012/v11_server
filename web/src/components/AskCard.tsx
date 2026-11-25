import React, { useState, useEffect } from 'react';
import { Sparkles, Send, Copy, Check, AlertCircle, LoaderCircle, RotateCcw } from 'lucide-react';
import { marked } from 'marked';
import { sendAction, subscribeAction } from '../lib/firebase';
import { ActionRequest } from '../lib/types';

function sanitizeHtml(html: string): string {
  if (!html) return '';
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.querySelectorAll('script,iframe,object,embed,style,link,meta,base,form').forEach((el) => el.remove());
  doc.body.querySelectorAll('*').forEach((el) => {
    for (const attr of Array.from(el.attributes)) {
      const name = attr.name.toLowerCase();
      const val = attr.value.trim();
      if (name.startsWith('on') || name === 'srcdoc') {
        el.removeAttribute(attr.name);
        continue;
      }
      if ((name === 'href' || name === 'src' || name === 'xlink:href') && !/^(https?:\/\/|\/|#)/i.test(val)) {
        el.removeAttribute(attr.name);
      }
    }
  });
  return doc.body.innerHTML;
}

const quickPrompts = [
  'สรุปข่าวทองวันนี้ให้หน่อย',
  'ภาพรวมข่าวเด่นวันนี้มีอะไรบ้าง',
  'เทรนด์ AI ล่าสุดที่น่าสนใจ',
];

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

  const submitQuery = async (queryText: string) => {
    const q = queryText.trim();
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
      setError(err?.message || 'ไม่สามารถส่งคำถามได้');
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await submitQuery(prompt);
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
    <section className="ask-panel" aria-labelledby="ask-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">02 · ผู้ช่วย AI</span>
          <h2 id="ask-title">
            <Sparkles style={{ color: 'var(--accent)' }} />
            <span>ถาม-ตอบ AI ด่วน</span>
          </h2>
        </div>
        <div className="model-switch" role="group" aria-label="เลือกโมเดล AI">
          {(['auto', 'groq', 'gemini'] as const).map((m) => (
            <button
              key={m}
              type="button"
              className={model === m ? 'active' : ''}
              onClick={() => setModel(m)}
            >
              {m === 'auto' ? 'Auto' : m === 'groq' ? 'Groq' : 'Gemini'}
            </button>
          ))}
        </div>
      </div>

      <div className="prompt-chips">
        {quickPrompts.map((chip) => (
          <button
            key={chip}
            type="button"
            className="prompt-chip"
            disabled={loading}
            onClick={() => {
              setPrompt(chip);
              void submitQuery(chip);
            }}
          >
            {chip}
          </button>
        ))}
      </div>

      <form className="ask-form" onSubmit={handleSubmit}>
        <input
          type="text"
          className="ask-input"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="พิมพ์คำถามที่ต้องการถาม AI..."
          disabled={loading}
        />
        <button
          type="submit"
          className="ask-submit"
          disabled={loading || !prompt.trim()}
        >
          {loading ? <LoaderCircle className="spin" /> : <Send />}
          <span>{loading ? 'กำลังคิด…' : 'ส่งคำถาม'}</span>
        </button>
      </form>

      {error && (
        <div className="ask-alert" role="alert">
          <AlertCircle />
          <span>{error}</span>
        </div>
      )}

      {trackedAction?.status === 'completed' && trackedAction.result && (
        <div className="ask-result">
          <div className="ask-result-bar">
            <div className="ask-result-meta">
              <Check />
              <span>ตอบกลับจาก {model === 'auto' ? 'Fast AI' : model.toUpperCase()}</span>
            </div>
            <div className="ask-result-actions">
              <button type="button" className="ask-mini-btn" onClick={handleCopy}>
                {copied ? <Check style={{ color: 'var(--green)' }} /> : <Copy />}
                <span>{copied ? 'คัดลอกแล้ว' : 'คัดลอก'}</span>
              </button>
              <button type="button" className="ask-mini-btn" onClick={handleClear}>
                <RotateCcw />
                <span>ล้าง</span>
              </button>
            </div>
          </div>
          <div
            className="ask-result-body markdown-body"
            dangerouslySetInnerHTML={{
              __html: sanitizeHtml(marked.parse(trackedAction.result, { gfm: true, breaks: true }) as string)
            }}
          />
        </div>
      )}
    </section>
  );
};
