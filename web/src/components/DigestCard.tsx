import React, { useState } from 'react';
import { Newspaper, Coins, TrendingUp, Sparkles, ChevronDown, ChevronUp } from 'lucide-react';
import { PublicDigest } from '../lib/types';

interface DigestCardProps {
  digest: PublicDigest;
}

export const DigestCard: React.FC<DigestCardProps> = ({ digest }) => {
  const [expanded, setExpanded] = useState(false);
  const { gold, stocks, summary_md, updated_at } = digest;

  const formatCurrency = (val: number) => {
    return new Intl.NumberFormat('th-TH').format(val);
  };

  return (
    <div className="digest-panel">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center space-x-2">
          <Newspaper className="w-4 h-4 text-[#0066cc]" />
          <h2 className="text-sm font-semibold text-gray-900 dark:text-white uppercase tracking-wider">
            สรุปประจำวัน (AI & การเงิน)
          </h2>
        </div>
        <span className="text-[11px] text-gray-400 dark:text-gray-500">
          รอบล่าสุด: {new Date(updated_at).toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' })}
        </span>
      </div>

      {/* Gold Quote Highlight */}
      {gold && (
        <div className="mb-4 bg-linear-to-br from-amber-500/10 via-amber-500/5 to-transparent border border-amber-500/20 rounded-2xl p-4">
          <div className="flex items-center justify-between mb-2">
            <span className="flex items-center space-x-1.5 text-xs font-semibold text-amber-700 dark:text-amber-400">
              <Coins className="w-4 h-4" />
              <span>ราคาทองคำวันนี้</span>
            </span>
            <span className="text-xs font-mono text-gray-600 dark:text-gray-400">
              Spot: ${gold.world_spot} /oz
            </span>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="bg-white/80 dark:bg-gray-800/70 rounded-xl p-2.5 text-center">
              <div className="text-[11px] text-gray-500 dark:text-gray-400">ทองคำแท่งรับซื้อ</div>
              <div className="text-lg font-bold text-gray-900 dark:text-white mt-0.5">
                ฿{formatCurrency(gold.bar_buy)}
              </div>
            </div>
            <div className="bg-white/80 dark:bg-gray-800/70 rounded-xl p-2.5 text-center">
              <div className="text-[11px] text-gray-500 dark:text-gray-400">ทองคำแท่งขายออก</div>
              <div className="text-lg font-bold text-amber-600 dark:text-amber-400 mt-0.5">
                ฿{formatCurrency(gold.bar_sell)}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Stocks section if available */}
      {stocks && stocks.length > 0 && (
        <div className="mb-4">
          <div className="flex items-center space-x-1.5 text-xs font-semibold text-gray-700 dark:text-gray-300 mb-2">
            <TrendingUp className="w-3.5 h-3.5 text-blue-500" />
            <span>พาดหัวข่าวตลาดหุ้น</span>
          </div>
          <div className="space-y-1.5">
            {stocks.map((item, idx) => (
              <div
                key={idx}
                className="text-xs text-gray-700 dark:text-gray-300 bg-gray-50 dark:bg-gray-800/30 px-3 py-2 rounded-xl flex items-center justify-between"
              >
                <span className="line-clamp-1">{item.title}</span>
                {item.source && (
                  <span className="text-[10px] text-gray-400 ml-2 font-mono">{item.source}</span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Summary Markdown preview & toggle */}
      {summary_md && (
        <div className="pt-3 border-t border-gray-100 dark:border-gray-800/60">
          <div className="flex items-center justify-between mb-2">
            <span className="flex items-center space-x-1.5 text-xs font-semibold text-gray-700 dark:text-gray-300">
              <Sparkles className="w-3.5 h-3.5 text-purple-500" />
              <span>บทวิเคราะห์สรุป AI</span>
            </span>
            <button
              onClick={() => setExpanded(!expanded)}
              className="text-xs text-[#0066cc] hover:underline flex items-center space-x-0.5 font-medium"
            >
              <span>{expanded ? 'ย่อเนื้อหา' : 'อ่านฉบับเต็ม'}</span>
              {expanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
            </button>
          </div>

          <div
            className={`text-xs text-gray-600 dark:text-gray-300 leading-relaxed space-y-2 whitespace-pre-line ${
              !expanded ? 'line-clamp-3' : ''
            }`}
          >
            {summary_md}
          </div>
        </div>
      )}
    </div>
  );
};
