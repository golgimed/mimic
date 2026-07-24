export { buildServer } from "./core/server.js";
import { buildServer } from "./core/server.js";

function printBanner(port: number) {
  const color = process.stdout.isTTY;
  const purple = (s: string) => (color ? `\x1b[35m${s}\x1b[0m` : s);
  const dim = (s: string) => (color ? `\x1b[2m${s}\x1b[0m` : s);

  console.log(`${purple("MIMIC")}  ${dim("looks like the provider, bites different")}`);
  console.log(dim(`listening on http://localhost:${port}`));
}

async function main() {
  const app = await buildServer();
  const port = Number(process.env.PORT ?? 3000);
  await app.listen({ port, host: "0.0.0.0" });
  printBanner(port);
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
