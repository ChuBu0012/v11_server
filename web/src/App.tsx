import React, { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { DeviceStatusCard } from './components/DeviceStatusCard';
import { DigestCard } from './components/DigestCard';
import { ActionButtons } from './components/ActionButtons';
import { subscribeStatus, subscribeDigest } from './lib/firebase';
import { PublicStatus, PublicDigest } from './lib/types';
import { mockStatus, mockDigest } from './lib/mock-data';
import { Globe, Lock } from 'lucide-react';

export const App: React.FC = () => {
  const [darkMode, setDarkMode] = useState(() => {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  });

  const [status, setStatus] = useState<PublicStatus>(mockStatus);
  const [digest, setDigest] = useState<PublicDigest>(mockDigest);

  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [darkMode]);

  useEffect(() => {
    const unsubStatus = subscribeStatus((newStatus) => {
      if (newStatus) setStatus(newStatus);
    });
    const unsubDigest = subscribeDigest((newDigest) => {
      if (newDigest) setDigest(newDigest);
    });

    return () => {
      unsubStatus();
      unsubDigest();
    };
  }, []);

  return (
    <div className="max-w-3xl mx-auto px-4 py-8 sm:px-6 lg:px-8 space-y-6">
      {/* Header */}
      <Header
        darkMode={darkMode}
        onToggleTheme={() => setDarkMode(!darkMode)}
      />

      {/* Main Status & Telemetry */}
      <DeviceStatusCard status={status} />

      {/* Safe Actions */}
      <ActionButtons />

      {/* Daily Digest & Financials */}
      <DigestCard digest={digest} />

      {/* Privacy & Architecture Guarantee Footer */}
      <footer className="pt-6 border-t border-gray-200 dark:border-gray-800 text-center space-y-2">
        <div className="flex items-center justify-center space-x-4 text-xs text-gray-500 dark:text-gray-400">
          <span className="flex items-center space-x-1">
            <Globe className="w-3.5 h-3.5 text-blue-500" />
            <span>Tailscale-Free Public URL</span>
          </span>
          <span>·</span>
          <span className="flex items-center space-x-1">
            <Lock className="w-3.5 h-3.5 text-emerald-500" />
            <span>GPS & Quota Strictly Isolated</span>
          </span>
        </div>
        <p className="text-[11px] text-gray-400 dark:text-gray-500">
          Pocket Sentinel v2.1 Public Hub · Hosted on Vercel · Powered by Firestore Buffer & vivo v11
        </p>
      </footer>
    </div>
  );
};
