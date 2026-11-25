import React, { useEffect, useMemo, useState } from 'react';
import { Coins, TrendingUp, Sparkles, Bot, Landmark, Lightbulb, LayoutGrid } from 'lucide-react';
import { PublicDigest } from '../lib/types';

interface DigestCardProps {
  digest: PublicDigest;
}

type BriefSection = 'OVERVIEW' | 'AI' | 'POLITICS' | 'STOCKS' | 'GOLD' | 'TRIVIA';

const sectionMeta: Record<BriefSection, { label: string; icon: React.ElementType }> = {
  OVERVIEW: { label: 'ภาพรวม', icon: LayoutGrid },
  AI: { label: 'AI', icon: Bot },
  POLITICS: { label: 'การเมือง', icon: Landmark },
  STOCKS: { label: 'หุ้น', icon: TrendingUp },
  GOLD: { label: 'ทองคำ', icon: Coins },
  TRIVIA: { label: 'สาระ', icon: Lightbulb },
};

function parseBrief(markdown: string): Record<BriefSection, string[]> {
  const parsed: Record<BriefSection, string[]> = {
    OVERVIEW: [], AI: [], POLITICS: [], STOCKS: [], GOLD: [], TRIVIA: [],
  };

  let current: BriefSection = 'OVERVIEW';
  for (const rawLine of markdown.split('\n')) {
    const line = rawLine.trim();
    const marker = line.match(/^===SECTION:([A-Z]+)===$/);
    if (marker && marker[1] in parsed) {
      current = marker[1] as BriefSection;
      continue;
    }
    if (!line) continue;
    parsed[current].push(line.replace(/^[-*]\s*/, '').trim());
  }

  return parsed;
}

function renderInlineMarkdown(text: string) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return parts.map((part, index) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={index}>{part.slice(2, -2)}</strong>;
    }
    return <React.Fragment key={index}>{part}</React.Fragment>;
  });
}

export const DigestCard: React.FC<DigestCardProps> = ({ digest }) => {
  const [activeSection, setActiveSection] = useState<BriefSection>('OVERVIEW');
  const { gold, stocks, summary_md, updated_at } = digest;

  const sections = useMemo(() => {
    const parsed = parseBrief(summary_md || '');
    if (parsed.STOCKS.length === 0 && stocks && stocks.length > 0) {
      parsed.STOCKS = stocks.map((s) => (s.source ? `**${s.source}:** ${s.title}` : s.title));
    }
    return parsed;
  }, [summary_md, stocks]);

  const availableSections = (Object.keys(sectionMeta) as BriefSection[]).filter(
    (section) => sections[section].length > 0,
  );
  const activeLines = sections[activeSection];
  const formatCurrency = (val: number) => new Intl.NumberFormat('th-TH').format(val);

  useEffect(() => {
    if (availableSections.length > 0 && !availableSections.includes(activeSection)) {
      setActiveSection(availableSections[0]);
    }
  }, [availableSections, activeSection]);

  return (
    <section className="digest-panel" aria-labelledby="digest-title">
      <div className="panel-heading">
        <div>
          <span className="section-kicker">03 · ข่าวและตลาด</span>
          <h2 id="digest-title">สรุปข่าวประจำวัน</h2>
        </div>
        <span className="updated-label">
          อัปเดต {new Date(updated_at).toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' })}
        </span>
      </div>

      {gold && (
        <div className="gold-strip">
          <div className="gold-heading">
            <span><Coins /> ราคาทองคำ</span>
            <small>Spot ${gold.world_spot} / oz</small>
          </div>
          <div className="gold-values">
            <div><small>รับซื้อ</small><strong>฿{formatCurrency(gold.bar_buy)}</strong></div>
            <div><small>ขายออก</small><strong>฿{formatCurrency(gold.bar_sell)}</strong></div>
          </div>
        </div>
      )}

      {availableSections.length > 0 ? (
        <div className="brief-reader">
          <div className="brief-tabs" role="tablist" aria-label="หมวดสรุปข่าว">
            {availableSections.map((section) => {
              const Icon = sectionMeta[section].icon;
              return (
                <button
                  key={section}
                  type="button"
                  role="tab"
                  aria-selected={activeSection === section}
                  className={activeSection === section ? 'active' : ''}
                  onClick={() => setActiveSection(section)}
                >
                  <Icon />
                  <span>{sectionMeta[section].label}</span>
                </button>
              );
            })}
          </div>

          <div className="brief-content" role="tabpanel">
            {activeLines.map((line, index) => {
              const isOverview = activeSection === 'OVERVIEW';
              const overviewMatch = isOverview ? line.match(/^([^:]+):\s*(.+)$/) : null;

              if (overviewMatch) {
                const rawLabel = overviewMatch[1];
                const label = rawLabel.includes('หุ้น') ? 'หุ้นไทย/โลก'
                  : rawLabel.includes('AI') ? 'AI & Tech'
                  : rawLabel.includes('การเมือง') ? 'การเมือง'
                  : rawLabel.includes('สาระ') ? 'สาระน่ารู้'
                  : rawLabel.replace(/^[^ก-ฮA-Za-z]+\s*/, '');

                return (
                  <article className="overview-row" key={`${line}-${index}`}>
                    <span className="overview-index">{String(index + 1).padStart(2, '0')}</span>
                    <div>
                      <strong>{label}</strong>
                      <p>{renderInlineMarkdown(overviewMatch[2])}</p>
                    </div>
                  </article>
                );
              }

              return (
                <article className="brief-row" key={`${line}-${index}`}>
                  <span className="brief-bullet" />
                  <p>{renderInlineMarkdown(line)}</p>
                </article>
              );
            })}
          </div>
        </div>
      ) : (
        <div className="brief-empty">
          <Sparkles />
          <span>ยังไม่มีสรุปข่าวประจำวัน</span>
        </div>
      )}
    </section>
  );
};
