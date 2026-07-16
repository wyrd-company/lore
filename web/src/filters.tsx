/*
---
relationships:
  implements: system
---
*/
export function FilterPanel({ activeCount, children }: { activeCount: number; children: React.ReactNode }) {
  return <details className="filter-panel"><summary><svg aria-hidden="true" className="filter-panel__icon" viewBox="0 0 20 20"><path d="M3 4h14l-5.5 6.2V15l-3 1v-5.8L3 4Z" /></svg><span>Filters</span>{activeCount > 0 && <span className="filter-panel__count">{activeCount}</span>}<span className="filter-panel__chevron">⌄</span></summary><div className="filter-panel__body">{children}</div></details>;
}

export function FilterGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return <fieldset className="facet-group"><legend>{title}</legend><div className="facet-stack">{children}</div></fieldset>;
}
