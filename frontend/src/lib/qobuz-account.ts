import type { QobuzAccount } from '@/types/api';

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          GetQobuzAccount?: () => Promise<QobuzAccount>;
          QobuzLogin?: (
            identifier: string,
            password: string,
          ) => Promise<QobuzAccount>;
          QobuzLoginWithToken?: (
            token: string,
            userID: number,
          ) => Promise<QobuzAccount>;
          StartQobuzOAuth?: () => Promise<string>;
          CancelQobuzOAuth?: () => Promise<void>;
          QobuzLogout?: () => Promise<void>;
        };
      };
    };
    runtime?: {
      EventsOn?: (
        eventName: string,
        callback: (...args: any[]) => void,
      ) => () => void;
      EventsOnce?: (
        eventName: string,
        callback: (...args: any[]) => void,
      ) => void;
      EventsOff?: (eventName: string) => void;
    };
  }
}

export async function getQobuzAccount(): Promise<QobuzAccount> {
  if (typeof window !== 'undefined' && window.go?.main?.App?.GetQobuzAccount) {
    return await window.go.main.App.GetQobuzAccount();
  }
  return {
    user_id: 0,
    user_auth_token: '',
    email: '',
    display_name: '',
    subscription: '',
    connected: false,
  };
}

export async function loginQobuz(
  identifier: string,
  password: string,
): Promise<QobuzAccount> {
  if (typeof window === 'undefined' || !window.go?.main?.App?.QobuzLogin) {
    throw new Error('Qobuz login is not available');
  }
  return await window.go.main.App.QobuzLogin(identifier, password);
}

export async function loginQobuzWithToken(
  token: string,
  userID: number = 0,
): Promise<QobuzAccount> {
  if (
    typeof window === 'undefined' ||
    !window.go?.main?.App?.QobuzLoginWithToken
  ) {
    throw new Error('Qobuz token login is not available');
  }
  return await window.go.main.App.QobuzLoginWithToken(token, userID);
}

export async function startQobuzOAuth(): Promise<string> {
  if (typeof window === 'undefined' || !window.go?.main?.App?.StartQobuzOAuth) {
    throw new Error('Qobuz OAuth is not available');
  }
  return await window.go.main.App.StartQobuzOAuth();
}

export async function cancelQobuzOAuth(): Promise<void> {
  if (typeof window !== 'undefined' && window.go?.main?.App?.CancelQobuzOAuth) {
    await window.go.main.App.CancelQobuzOAuth();
  }
}

export async function logoutQobuz(): Promise<void> {
  if (typeof window !== 'undefined' && window.go?.main?.App?.QobuzLogout) {
    await window.go.main.App.QobuzLogout();
  }
}

