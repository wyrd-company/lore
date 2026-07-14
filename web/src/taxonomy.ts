import type { TermSummary } from "./types";

const svgNamespace = "http://www.w3.org/2000/svg";

export interface TaxonomyMatch {
  start: number;
  end: number;
  kind: "tag" | "term";
  name: string;
}

interface Candidate {
  label: string;
  kind: TaxonomyMatch["kind"];
  name: string;
}

export function taxonomyMatches(text: string, tags: string[], terms: TermSummary[]): TaxonomyMatch[] {
  const candidates = taxonomyCandidates(tags, terms);
  const normalized = text.toLocaleLowerCase();
  const matches: TaxonomyMatch[] = [];
  for (let index = 0; index < normalized.length;) {
    const candidate = candidates.find((item) => normalized.startsWith(item.label, index) && bounded(normalized, index, index + item.label.length));
    if (!candidate) {
      index++;
      continue;
    }
    matches.push({ start: index, end: index + candidate.label.length, kind: candidate.kind, name: candidate.name });
    index += candidate.label.length;
  }
  return matches;
}

export function linkTaxonomyText(root: HTMLElement, project: string, tags: string[], terms: TermSummary[]) {
  const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes: Text[] = [];
  let node: Node | null;
  while ((node = walker.nextNode())) {
    if (node instanceof Text && node.data.trim() && !node.parentElement?.closest("a,button,code,pre,script,style,textarea")) nodes.push(node);
  }
  for (const textNode of nodes) {
    const matches = taxonomyMatches(textNode.data, tags, terms);
    if (!matches.length) continue;
    const fragment = root.ownerDocument.createDocumentFragment();
    let offset = 0;
    for (const match of matches) {
      fragment.append(textNode.data.slice(offset, match.start));
      const link = textNode.parentElement?.namespaceURI === svgNamespace
        ? root.ownerDocument.createElementNS(svgNamespace, "a")
        : root.ownerDocument.createElement("a");
      link.setAttribute("class", `taxonomy-link taxonomy-link--${match.kind}`);
      link.setAttribute("data-taxonomy", match.name);
      link.textContent = textNode.data.slice(match.start, match.end);
      link.setAttribute("href", match.kind === "term"
        ? `/${encodeURIComponent(project)}/terms/${encodeURIComponent(match.name)}`
        : `/${encodeURIComponent(project)}/search?q=${encodeURIComponent(match.name)}&tag=${encodeURIComponent(match.name)}`);
      fragment.append(link);
      offset = match.end;
    }
    fragment.append(textNode.data.slice(offset));
    textNode.replaceWith(fragment);
  }
}

function taxonomyCandidates(tags: string[], terms: TermSummary[]): Candidate[] {
  const candidates: Candidate[] = [];
  const labels = new Set<string>();
  const add = (label: string, kind: Candidate["kind"], name: string) => {
    const normalized = label.trim().toLocaleLowerCase();
    if (!normalized || labels.has(normalized)) return;
    labels.add(normalized);
    candidates.push({ label: normalized, kind, name });
  };
  for (const term of terms) {
    add(term.title, "term", term.name);
    add(term.name, "term", term.name);
  }
  for (const tag of tags) add(tag, "tag", tag);
  return candidates.sort((left, right) => {
    const byLength = right.label.length - left.label.length;
    if (byLength !== 0) return byLength;
    if (left.kind !== right.kind) return left.kind === "term" ? -1 : 1;
    return left.label.localeCompare(right.label);
  });
}

function bounded(value: string, start: number, end: number): boolean {
  return (start === 0 || !wordCharacter(value[start - 1])) && (end === value.length || !wordCharacter(value[end]));
}

function wordCharacter(value: string | undefined): boolean {
  return Boolean(value && /[\p{L}\p{N}_]/u.test(value));
}
