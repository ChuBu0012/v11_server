import React, { useState } from 'react';
import { Newspaper, Coins, TrendingUp, Sparkles, ChevronDown, ChevronUp } from 'lucide-react';
import { PublicDigest } from '../lib/types';

interface DigestCardProps {
  digest: PublicDigest;
}

export const DigestCard: React.FC<DigestCardProps> = ({ digest }) => {
  const [expanded, setExpanded] = useState(false);
  const { gold, stocks, news, summary_md, updated_at } = digest;

  const formatCurrency = (val: number) => new Intl.NumberFormat('th-TH').format(val);

  return (
    <section className="digest-panel" aria-labelledby="digest-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">03 / INTELLIGENCE</span>
          <h2 id="digest-title">Daily brief</h2>
        </div>
        <span className="updated-label">
          {new Date(updated_at).toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' })}
        </span>
      </div>

      {gold && (
        <div className="gold-strip">
          <div className="gold-heading">
            <span><Coins /> Gold</span>
            <small>${gold.world_spot} / oz</small>
          </div>
          <div className="gold-values">
            <div><small>BUY</small><strong>฿{formatCurrency(gold.bar_buy)}</strong></div>
            <div><small>SELL</small><strong>฿{formatCurrency(gold.bar_sell)}</strong></div>
          </div>
        </div>
      )}

      <div className="brief-grid">
        {stocks && stocks.length > 0 && (
          <div className="brief-block">
            <div className="brief-label"><TrendingUp /> Market</div>
            <div className="headline-list">
              {stocks.slice(0, 3).map((item, idx) => (
                <article key={`${item.title}-${idx}`}>
                  <span>{String(idx + 1).padStart(2, '0')}</span>
                  <div><strong>{item.title}</strong>{item.source && <small>{item.source}</small>}</div>
                </article>
              ))}
            </div>
          </div>
        )}

        {news && news.length > 0 && (
          <div className="brief-block">
            <div className="brief-label"><Newspaper /> Signal</div>
            <div className="headline-list">
              {news.slice(0, 3).map((item, idx) => (
                <article key={`${item.category}-${idx}`}>
                  <span>{item.category.slice(0, 2).toUpperCase()}</span>
                  <div><strong>{item.summary}</strong></div>
                </article>
              ))}
            </div>
          </div>
        )}
      </div>

      {summary_md && (
        <div className="ai-summary">
          <button onClick={() => setExpanded(!expanded)}>
            <span><Sparkles /> AI Analysis</span>
            <span>{expanded ? 'Collapse' : 'Read full'}{expanded ? <ChevronUp /> : <ChevronDown />}</span>
          </button>
          <div className={expanded ? 'summary-copy expanded' : 'summary-copy'}>{summary_md}</div>
        </div>
      )}
    </section>
  );
};
