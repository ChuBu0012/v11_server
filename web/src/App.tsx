import React, { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { DeviceStatusCard } from './components/DeviceStatusCard';
import { DigestCard } from './components/DigestCard';
import { AskCard } from './components/AskCard';
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
    <div className="app-shell">
      <Header
        darkMode={darkMode}
        onToggleTheme={() => setDarkMode(!darkMode)}
        status={status}
      />

      <main className="dashboard-layout">
        <div className="operation-stream">
          <DeviceStatusCard status={status} />
          <AskCard />
          <DigestCard digest={digest} />
        </div>

        <aside className="command-rail">
          <ActionButtons />
        </aside>
      </main>

      <footer className="site-footer">
        <div className="privacy-row">
          <span><Globe /> Public Hub</span>
          <span><Lock /> ไม่เปิดเผยพิกัด GPS และโควตาส่วนตัว</span>
        </div>
        <p>Pocket Sentinel · vivo 1804</p>
      </footer>
    </div>
  );
};
