// The commands that install an agent for an environment (design 7), from
// its enrollment token and the hub's address.

export type Commands = { systemd: string; docker: string; compose: string; local?: string };

const image = "ghcr.io/cowlhq/lotse";

// dev says the hub is a development build, most likely this repository's
// compose stack, whose own agent gets a command of its own.
export function installCommands(hub: string, token: string, dev = false): Commands {
  const url = new URL(hub);
  const local = url.hostname === "localhost" || url.hostname === "127.0.0.1";
  // Plain HTTP to a hub on another machine needs saying so (decision 0006).
  const insecure = url.protocol === "http:" && !local;
  const flag = insecure ? " --insecure" : "";
  const env = insecure ? "\n  -e LOTSE_INSECURE=true \\" : "";

  const systemd = `curl -fsSL https://lotse.sh/install | sudo sh -s -- agent --hub ${hub} --token ${token}${flag}`;

  const docker = `docker run -d --name lotse-agent --restart unless-stopped --user 0 \\
  -e LOTSE_HUB=${hub} \\
  -e LOTSE_TOKEN=${token} \\${env}
  -e LOTSE_HOST_ROOT=/host \\
  -v /:/host:ro,rslave \\
  -v /var/run/docker.sock:/var/run/docker.sock:ro \\
  -v lotse-agent:/var/lib/lotse \\
  ${image} start agent`;

  const compose = `services:
  lotse-agent:
    image: ${image}
    command: ["start", "agent"]
    restart: unless-stopped
    user: "0"
    environment:
      LOTSE_HUB: ${hub}
      LOTSE_TOKEN: ${token}${insecure ? "\n      LOTSE_INSECURE: \"true\"" : ""}
      LOTSE_HOST_ROOT: /host
    volumes:
      - /:/host:ro,rslave
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - lotse-agent:/var/lib/lotse

volumes:
  lotse-agent:`;

  return {
    systemd,
    docker,
    compose,
    // The compose file of this repository runs its agent in a profile.
    local: local || dev ? `LOTSE_TOKEN=${token} docker compose --profile agent up -d` : undefined,
  };
}

// The commands that run a probe under its name. Probes are updated by
// running the command again (decision 0009).
export function probeCommands(hub: string, token: string, name: string, location: string): { systemd: string; docker: string } {
  const url = new URL(hub);
  const local = url.hostname === "localhost" || url.hostname === "127.0.0.1";
  const insecure = url.protocol === "http:" && !local;
  const where = location ? ` --location ${quote(location)}` : "";

  const systemd = `curl -fsSL https://lotse.sh/install | sudo sh -s -- probe --hub ${hub} --token ${token} --name ${name}${where}${insecure ? " --insecure" : ""}`;

  const docker = `docker run -d --name lotse-probe --restart unless-stopped \\
  -e LOTSE_HUB=${hub} \\
  -e LOTSE_TOKEN=${token} \\${insecure ? "\n  -e LOTSE_INSECURE=true \\" : ""}
  -e LOTSE_PROBE_NAME=${name} \\${location ? `\n  -e LOTSE_PROBE_LOCATION=${quote(location)} \\` : ""}
  -v lotse-probe:/var/lib/lotse \\
  ${image} probe`;

  return { systemd, docker };
}

// quote makes text one word for the shell.
function quote(s: string): string {
  return /^[\w.,/-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`;
}
