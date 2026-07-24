import type { FastifyInstance } from "fastify";

export interface DashboardItem {
  provider: string;
  type: string;
  id: string;
  status: string;
  createdAt: string;
}

export interface ItemDetail {
  provider: string;
  payload: unknown;
  webhookDeliveries: Array<{
    url: string;
    payload: unknown;
    status: string;
    responseCode: number | null;
    createdAt: string;
  }>;
}

export interface Provider {
  /** Also used as the route prefix (e.g. "zenvia" -> /zenvia/...). */
  name: string;
  register(app: FastifyInstance): void | Promise<void>;
  listItems?(): DashboardItem[];
  getItemDetail?(id: string): ItemDetail | undefined;
}

const providers = new Map<string, Provider>();

export function registerProvider(provider: Provider): void {
  providers.set(provider.name, provider);
}

export function getProvider(name: string): Provider | undefined {
  return providers.get(name);
}

export function getAllProviders(): Provider[] {
  return [...providers.values()];
}

export function getEnabledProviders(): Provider[] {
  const enabledNames = getEnabledProviderNames();
  return getAllProviders().filter((p) => enabledNames.includes(p.name));
}

function getEnabledProviderNames(): string[] {
  const raw = process.env.MIMIC_PROVIDERS;
  if (!raw || raw.trim() === "") {
    return getAllProviders().map((p) => p.name);
  }
  return raw
    .split(",")
    .map((name) => name.trim())
    .filter(Boolean);
}
