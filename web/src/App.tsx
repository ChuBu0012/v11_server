import React, { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { DeviceStatusCard } from './components/DeviceStatusCard';
import { DigestCard } from './components/DigestCard';
import { AskCard } from './components/AskCard';
import { ActionButtons } from './components/ActionButtons';
import { WorkspacePublicView } from './components/WorkspacePublicView';
import { subscribeStatus, subscribeDigest, subscribeWorkspace } from './lib/firebase';
import { PublicStatus, PublicDigest, PublicWorkspaceData } from './lib/types';
import { mockStatus, mockDigest, mockWorkspace } from './lib/mock-data';
import { Globe, Lock } from 'lucide-react';

export const App: React.FC = () => {
  const [darkMode, setDarkMode] = useState(() => {
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  });

  const [status, setStatus] = useState<PublicStatus>(mockStatus);
  const [digest, setDigest] = useState<PublicDigest>(mockDigest);
  const [workspace, setWorkspace] = useState<PublicWorkspaceData>(mockWorkspace);

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
    const unsubWorkspace = subscribeWorkspace((newWs) => {
      if (newWs) setWorkspace(newWs);
    });

    return () => {
      unsubStatus();
      unsubDigest();
      unsubWorkspace();
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
          <WorkspacePublicView workspace={workspace} />
          <DigestCard digest={digest} />
        </div>

        <aside className="command-rail">
          <ActionButtons />
        </aside>
      </main>

      <footer className="site-footer">
        <div className="privacy-row">
          <span><Globe /> Public edge</span>
          <span><Lock /> No GPS or private quota exposure</span>
        </div>
        <p>Pocket Sentinel / Public operations surface</p>
      </footer>
    </div>
  );
};
