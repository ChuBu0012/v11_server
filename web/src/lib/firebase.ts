import { initializeApp, getApps, getApp } from 'firebase/app';
import { 
  getFirestore, 
  doc, 
  onSnapshot, 
  collection, 
  addDoc, 
  serverTimestamp,
  Firestore 
} from 'firebase/firestore';
import { PublicStatus, PublicDigest, ActionRequest } from './types';
import { mockStatus, mockDigest } from './mock-data';

const firebaseConfig = {
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
  storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
  messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
  appId: import.meta.env.VITE_FIREBASE_APP_ID,
};

// Auto-detect whether Firebase config is configured or we should run in mock mode
export const isMockMode = !firebaseConfig.apiKey || firebaseConfig.apiKey.includes('your_');

let db: Firestore | null = null;
if (!isMockMode) {
  try {
    const app = getApps().length > 0 ? getApp() : initializeApp(firebaseConfig);
    db = getFirestore(app);
  } catch (err) {
    console.warn('[firebase] Initialization failed, falling back to mock mode:', err);
  }
}

// Subscribe to live status updates
export function subscribeStatus(callback: (status: PublicStatus) => void): () => void {
  if (isMockMode || !db) {
    callback(mockStatus);
    return () => {};
  }

  const docRef = doc(db, 'sentinel_public', 'status');
  return onSnapshot(
    docRef,
    (snap) => {
      if (snap.exists()) {
        callback(snap.data() as PublicStatus);
      } else {
        callback(mockStatus);
      }
    },
    (err) => {
      console.warn('[firestore] Failed to fetch status, using fallback:', err);
      callback(mockStatus);
    }
  );
}

// Subscribe to live digest updates
export function subscribeDigest(callback: (digest: PublicDigest) => void): () => void {
  if (isMockMode || !db) {
    callback(mockDigest);
    return () => {};
  }

  const docRef = doc(db, 'sentinel_public', 'digest');
  return onSnapshot(
    docRef,
    (snap) => {
      if (snap.exists()) {
        callback(snap.data() as PublicDigest);
      } else {
        callback(mockDigest);
      }
    },
    (err) => {
      console.warn('[firestore] Failed to fetch digest, using fallback:', err);
      callback(mockDigest);
    }
  );
}

// Dispatch an action to Firestore
export async function sendAction(
  action: string,
  payload?: { prompt?: string; model?: string }
): Promise<string> {
  if (isMockMode || !db) {
    await new Promise((r) => setTimeout(r, 800));
    return `preview-${action}-${Date.now()}`;
  }

  const colRef = collection(db, 'sentinel_actions');
  const docData: Record<string, any> = {
    action,
    status: 'pending',
    requested_at: new Date().toISOString(),
    created_at: serverTimestamp(),
  };
  if (payload?.prompt) {
    docData.prompt = payload.prompt;
  }
  if (payload?.model) {
    docData.model = payload.model;
  }
  const res = await addDoc(colRef, docData);
  return res.id;
}

export function subscribeAction(id: string, callback: (action: ActionRequest | null) => void): () => void {
  if (!db || id.startsWith('preview-')) {
    callback({
      id,
      action: id.includes('digest') ? 'refresh_digest' : id.includes('ask') ? 'ask' : 'ping',
      status: 'completed',
      requested_at: new Date().toISOString(),
      completed_at: new Date().toISOString(),
      result: id.includes('ask') 
        ? 'คำตอบจำลอง: ท้องฟ้าเป็นสีฟ้าเนื่องจากการกระเจิงแบบเรย์ลี (Rayleigh scattering) เมื่อแสงอาทิตย์เดินทางผ่านชั้นบรรยากาศ'
        : 'Preview completed',
    });
    return () => {};
  }

  return onSnapshot(doc(db, 'sentinel_actions', id), (snap) => {
    callback(snap.exists() ? ({ id: snap.id, ...snap.data() } as ActionRequest) : null);
  }, (err) => {
    console.warn('[firestore] Failed to track action:', err);
    callback(null);
  });
}
