import { registerProvider } from "../core/registry.js";
import { zenviaProvider } from "./zenvia/provider.js";
import { integraIcpProvider } from "./integraicp/provider.js";

export function registerAllProviders(): void {
  registerProvider(zenviaProvider);
  registerProvider(integraIcpProvider);
}
