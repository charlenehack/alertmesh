// Thin wrapper over the shared LogStoreForm; OpenSearch and
// Elasticsearch share the HTTP query DSL + Basic-Auth shape so the only
// per-kind difference is labels / placeholders.

import { LogStoreForm } from './LogStoreForm'

export interface ElasticFormProps {
  editing: boolean
}

export function ElasticForm({ editing }: ElasticFormProps) {
  return <LogStoreForm kind="elastic" editing={editing} />
}
