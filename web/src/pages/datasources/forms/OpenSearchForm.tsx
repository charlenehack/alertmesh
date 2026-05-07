// Thin wrapper over the shared LogStoreForm; OpenSearch and
// Elasticsearch share the HTTP query DSL + Basic-Auth shape so the only
// per-kind difference is labels / placeholders.

import { LogStoreForm } from './LogStoreForm'

export interface OpenSearchFormProps {
  editing: boolean
}

export function OpenSearchForm({ editing }: OpenSearchFormProps) {
  return <LogStoreForm kind="opensearch" editing={editing} />
}
