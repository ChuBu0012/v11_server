import React from 'react';
import { Sun, Moon, ShieldCheck, Sparkles } from 'lucide-react';
import { isMockMode } from '../lib/firebase';

interface HeaderProps {
  darkMode: boolean;
  onToggleTheme: () => void;
}

export const Header: React.FC<HeaderProps> = ({ darkMode, onToggleTheme }) => {
  return (
    <header className="flex flex-col sm:flex-row sm:items-center justify-between pb-6 border-b border-gray-200 dark:border-gray-800 gap-4">
      <div className="flex items-center space-x-3">
        <div className="relative flex items-center justify-center w-10 h-10 rounded-2xl bg-[#0066cc]/10 dark:bg-[#0066cc]/20 text-[#0066cc]">
          <ShieldCheck className="w-5 h-5" />
          <span className="absolute -top-0.5 -right-0.5 flex h-3 w-3">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-3 w-3 bg-emerald-500"></span>
          </span>
        </div>
        <div>
          <div className="flex items-center space-x-2">
            <h1 className="text-xl font-bold tracking-tight text-gray-900 dark:text-white">
              Pocket Sentinel
            </h1>
            <span className="text-xs px-2 py-0.5 font-medium rounded-full bg-blue-100 dark:bg-blue-900/40 text-blue-800 dark:text-blue-300">
              Public Hub
            </span>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400">
            vivo 1804 (v11) · Tailscale-Free Real-time Telemetry
          </p>
        </div>
      </div>

      <div className="flex items-center space-x-3 self-end sm:self-auto">
        {isMockMode ? (
          <span className="flex items-center space-x-1.5 text-xs px-2.5 py-1 rounded-full bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-300 font-medium">
            <Sparkles className="w-3.5 h-3.5 text-amber-500" />
            <span>Preview Mode</span>
          </span>
        ) : (
          <span className="flex items-center space-x-1.5 text-xs px-2.5 py-1 rounded-full bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 text-emerald-800 dark:text-emerald-300 font-medium">
            <span className="w-2 h-2 rounded-full bg-emerald-500"></span>
            <span>Live Sync</span>
          </span>
        )}

        <button
          onClick={onToggleTheme}
          aria-label="สลับธีม"
          className="p-2 rounded-xl bg-gray-100 hover:bg-gray-200 dark:bg-gray-800 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300 transition-colors"
        >
          {darkMode ? <Sun className="w-4 h-4 text-amber-400" /> : <Moon className="w-4 h-4 text-gray-600" />}
        </button>
      </div>
    </header>
  );
};
