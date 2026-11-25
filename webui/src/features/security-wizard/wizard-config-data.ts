import type { SecurityClusterSettings } from "../../types";
import { isWildcardHost, joinHostPort, shiftedPort, splitHostPort, urlHost, validPort } from "./wizard-address-data";
import type { WizardNodeRole } from "./wizard-nodes-data";

export const clusterPortOffset = 10;

export interface WizardConfigNode {
  key: string;
  label: string;
  role: WizardNodeRole;
  nodeURL: string;
  cluster: SecurityClusterSettings;
}

export interface ClusterSettingChange {
  setting: string;
  current: string;
  proposed: string;
}

export interface NodeClusterPlan {
  nodeKey: string;
  label: string;
  port: string;
  yaml: string;
  changes: ClusterSettingChange[];
  problem: string;
}

interface ProposedSettings {
  bind: string;
  publicURL: string;
}

export function defaultClusterPort(cluster: SecurityClusterSettings): string {
  const configured = splitHostPort(cluster.bind).port;
  return validPort(configured) ? configured : shiftedPort(splitHostPort(cluster.server_bind).port, clusterPortOffset);
}

export function clusterPlans(nodes: readonly WizardConfigNode[], ports: Readonly<Record<string, string>>): NodeClusterPlan[] {
  const portFor = (node: WizardConfigNode): string => ports[node.key] ?? defaultClusterPort(node.cluster);
  const proposed = new Map(nodes.map(node => [node.key, proposedSettings(node, portFor(node))]));
  const masterURL = nodes.filter(node => node.role === "master").map(node => proposed.get(node.key)?.publicURL ?? "")[0] ?? "";
  const slaveURLs = nodes.filter(node => node.role === "slave").map(node => proposed.get(node.key)?.publicURL ?? "").filter(url => url !== "");
  return nodes.map(node => {
    const settings = proposed.get(node.key) ?? {bind: "", publicURL: ""};
    const entries = settingEntries(node, settings, node.role === "master" ? slaveURLs : masterURL);
    return {
      nodeKey: node.key,
      label: node.label,
      port: portFor(node),
      yaml: clusterYAML(entries),
      changes: entries.filter(entry => entry.current !== entry.proposed),
      problem: planProblem(node, portFor(node), settings)
    };
  });
}

function proposedSettings(node: WizardConfigNode, port: string): ProposedSettings {
  const host = publicHost(node);
  const bindHost = splitHostPort(node.cluster.bind || node.cluster.server_bind).host;
  return {
    bind: validPort(port) ? joinHostPort(bindHost, port) : "",
    publicURL: validPort(port) && host ? `https://${joinHostPort(host, port)}` : ""
  };
}

function publicHost(node: WizardConfigNode): string {
  const bindHost = splitHostPort(node.cluster.server_bind).host;
  return urlHost(node.cluster.public_url) || urlHost(node.nodeURL) || (isWildcardHost(bindHost) ? "" : bindHost);
}

function settingEntries(node: WizardConfigNode, settings: ProposedSettings, peers: string | readonly string[]): ClusterSettingChange[] {
  const entries: ClusterSettingChange[] = [
    {setting: "bind", current: node.cluster.bind, proposed: settings.bind},
    {setting: "public_url", current: node.cluster.public_url, proposed: settings.publicURL}
  ];
  if (typeof peers === "string") {
    entries.push({setting: "master_url", current: node.cluster.master_url, proposed: peers});
  } else {
    entries.push({setting: "slave_urls", current: yamlList(node.cluster.slave_urls), proposed: yamlList(peers)});
  }
  return entries;
}

function clusterYAML(entries: readonly ClusterSettingChange[]): string {
  const lines = entries.map(entry => `  ${entry.setting}: ${entry.setting === "slave_urls" ? entry.proposed : JSON.stringify(entry.proposed)}`);
  return ["cluster:", ...lines].join("\n");
}

function yamlList(values: readonly string[]): string {
  return `[${values.map(value => JSON.stringify(value)).join(", ")}]`;
}

function planProblem(node: WizardConfigNode, port: string, settings: ProposedSettings): string {
  if (!validPort(port)) {
    return "Enter a cluster port between 1 and 65535.";
  }
  if (splitHostPort(node.cluster.server_bind).port === port) {
    return "The cluster port must differ from the server.bind port.";
  }
  return settings.publicURL ? "" : "No host is known for this router; set cluster.public_url by hand.";
}
