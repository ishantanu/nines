import { isAlias, isCollection, isPair, parseAllDocuments } from "yaml";

export interface KubernetesResource {
  kind: string;
  name: string;
  namespace: string;
  service?: string;
  selector?: Record<string, string>;
  labels?: Record<string, string>;
  dependsOn?: string[];
  hosts?: string[];
  backends?: string[];
}
export interface KubernetesInventory {
  resources: KubernetesResource[];
  ignoredResources: number;
  unresolvedReferences: number;
}
const object = (v: unknown): Record<string, unknown> =>
  v && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
const field = (v: unknown, ...path: string[]): unknown =>
  path.reduce<unknown>((current, key) => object(current)[key], v);
const text = (v: unknown) => (typeof v === "string" ? v : "");
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const labels = (v: unknown): Record<string, string> =>
  Object.fromEntries(
    Object.entries(object(v)).filter(
      (entry): entry is [string, string] => typeof entry[1] === "string",
    ),
  );
const annotation = (v: unknown, key: string) =>
  text(field(v, "metadata", "annotations", key));
const supported = new Set([
  "Service",
  "Deployment",
  "StatefulSet",
  "DaemonSet",
  "Job",
  "CronJob",
  "Pod",
  "Ingress",
]);
export function kubernetesHost(value: string): string {
  value = value.trim();
  if (value.length > 2048 || /[\s$(){}]/.test(value)) return "";
  if (value.includes("://") || value.includes(":")) {
    try {
      const full = value.includes("://");
      const url = new URL(full ? value : `http://${value}`);
      if (
        !full &&
        (url.username ||
          url.password ||
          url.pathname !== "/" ||
          url.search ||
          url.hash)
      )
        return "";
      value = url.hostname;
    } catch {
      return "";
    }
  }
  value = value.toLowerCase().replace(/\.$/, "");
  return /^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$/.test(value) &&
    !value.includes("..")
    ? value
    : "";
}
// This projection runs in the browser. Raw workload manifests, inline credentials,
// Secret/ConfigMap references and full URLs never leave the editor.
export function projectKubernetes(
  raw: string,
  namespace: string,
): KubernetesInventory {
  if (
    !raw.trim() ||
    new TextEncoder().encode(raw).length > 2 << 20 ||
    !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(namespace)
  )
    throw new Error(
      "Provide Kubernetes YAML/JSON up to 2 MiB and a valid default namespace.",
    );
  const out: KubernetesInventory = {
    resources: [],
    ignoredResources: 0,
    unresolvedReferences: 0,
  };
  let nodes = 0,
    resources = 0;
  function check(node: unknown, depth = 0) {
    if (++nodes > 150000 || depth > 40 || isAlias(node))
      throw new Error(
        "Aliased or excessively nested Kubernetes YAML is unsupported.",
      );
    if (isCollection(node))
      for (const item of node.items) check(item, depth + 1);
    else if (isPair(node)) {
      check(node.key, depth + 1);
      check(node.value, depth + 1);
    }
  }
  function add(resource: unknown) {
    const kind = text(field(resource, "kind"));
    if (kind === "List" || kind.endsWith("List")) {
      for (const item of list(field(resource, "items"))) {
        const child = object(item);
        add(
          !text(child.kind) && kind !== "List"
            ? { ...child, kind: kind.slice(0, -4) }
            : item,
        );
      }
      return;
    }
    if (++resources > 1000)
      throw new Error("Kubernetes inventory exceeds 1000 resources.");
    if (!supported.has(kind)) {
      out.ignoredResources++;
      return;
    }
    const r: KubernetesResource = {
      kind,
      name: text(field(resource, "metadata", "name")),
      namespace: text(field(resource, "metadata", "namespace")) || namespace,
    };
    let service = annotation(resource, "nines.dev/service");
    let depends = annotation(resource, "nines.dev/depends-on");
    const hosts: string[] = [];
    if (kind === "Service") {
      r.selector = labels(field(resource, "spec", "selector"));
      const host = kubernetesHost(
        text(field(resource, "spec", "externalName")),
      );
      if (host) hosts.push(host);
    } else if (kind === "Ingress") {
      const backends = [
        text(field(resource, "spec", "defaultBackend", "service", "name")),
      ];
      for (const rule of list(field(resource, "spec", "rules")))
        for (const path of list(field(rule, "http", "paths")))
          backends.push(text(field(path, "backend", "service", "name")));
      r.backends = backends.filter(Boolean);
    } else {
      const pod =
        kind === "Pod"
          ? resource
          : kind === "CronJob"
            ? field(resource, "spec", "jobTemplate", "spec", "template")
            : field(resource, "spec", "template");
      r.labels = labels(field(pod, "metadata", "labels"));
      service = annotation(pod, "nines.dev/service") || service;
      depends = annotation(pod, "nines.dev/depends-on") || depends;
      for (const containerKind of ["containers", "initContainers"])
        for (const container of list(field(pod, "spec", containerKind))) {
          for (const env of list(field(container, "env"))) {
            if (
              !/(^|_)(HOST|HOSTNAME|URL|URI|ENDPOINT|ADDRESS|ADDR|DSN)$/i.test(
                text(field(env, "name")),
              )
            )
              continue;
            const host = kubernetesHost(text(field(env, "value")));
            if (host) hosts.push(host);
            else out.unresolvedReferences++;
          }
          if (list(field(container, "envFrom")).length)
            out.unresolvedReferences++;
        }
    }
    const reference = /^[a-zA-Z0-9][a-zA-Z0-9_./-]{0,199}$/;
    if (service) {
      if (reference.test(service)) r.service = service;
      else out.unresolvedReferences++;
    }
    if (depends)
      r.dependsOn = depends
        .split(",")
        .map((v) => v.trim())
        .filter((v) => {
          if (reference.test(v)) return true;
          out.unresolvedReferences++;
          return false;
        });
    if (hosts.length) r.hosts = hosts;
    out.resources.push(r);
  }
  try {
    const documents = parseAllDocuments(raw);
    for (const doc of documents) {
      if (doc.errors.length) throw new Error();
      check(doc.contents);
      if (doc.contents) add(doc.toJS({ maxAliasCount: 0 }));
    }
  } catch {
    throw new Error(
      "Invalid, oversized or aliased Kubernetes resource YAML. Export only the supported resource kinds.",
    );
  }
  if (!out.resources.length)
    throw new Error("No supported Kubernetes resources found.");
  return out;
}
