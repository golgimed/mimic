import { getEnabledProviders, getProvider, type DashboardItem, type ItemDetail } from "../../core/registry.js";

export function listItems(): DashboardItem[] {
  const items = getEnabledProviders().flatMap((p) => p.listItems?.() ?? []);
  return items.sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
}

export function getItemDetail(provider: string, id: string): ItemDetail | undefined {
  return getProvider(provider)?.getItemDetail?.(id);
}
