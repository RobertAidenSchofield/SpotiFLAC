import type { QobuzAccount } from '@/types/api';
import {
  GetQobuzAccount,
  QobuzLogin,
  QobuzLoginWithToken,
  StartQobuzOAuth,
  CancelQobuzOAuth,
  QobuzLogout,
} from '../../wailsjs/go/main/App';

export async function getQobuzAccount(): Promise<QobuzAccount> {
  try {
    const acc = await GetQobuzAccount();
    return {
      user_id: acc?.user_id ?? 0,
      user_auth_token: acc?.user_auth_token ?? '',
      email: acc?.email ?? '',
      display_name: acc?.display_name ?? '',
      subscription: acc?.subscription ?? '',
      country_code: acc?.country_code,
      connected: acc?.connected ?? false,
    };
  } catch {
    return {
      user_id: 0,
      user_auth_token: '',
      email: '',
      display_name: '',
      subscription: '',
      connected: false,
    };
  }
}

export async function loginQobuz(
  identifier: string,
  password: string,
): Promise<QobuzAccount> {
  const acc = await QobuzLogin(identifier, password);
  return {
    user_id: acc.user_id,
    user_auth_token: acc.user_auth_token,
    email: acc.email,
    display_name: acc.display_name,
    subscription: acc.subscription,
    country_code: acc.country_code,
    connected: acc.connected,
  };
}

export async function loginQobuzWithToken(
  token: string,
  userID: number = 0,
): Promise<QobuzAccount> {
  const acc = await QobuzLoginWithToken(token, userID);
  return {
    user_id: acc.user_id,
    user_auth_token: acc.user_auth_token,
    email: acc.email,
    display_name: acc.display_name,
    subscription: acc.subscription,
    country_code: acc.country_code,
    connected: acc.connected,
  };
}

export async function startQobuzOAuth(): Promise<string> {
  return await StartQobuzOAuth();
}

export async function cancelQobuzOAuth(): Promise<void> {
  await CancelQobuzOAuth();
}

export async function logoutQobuz(): Promise<void> {
  await QobuzLogout();
}

