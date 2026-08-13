import type {
  OrganizationRuntimeOverrideValues,
  OrganizationRuntimeSettings,
  OrganizationRuntimeSettingsPatch,
} from '#/lib/api/types'

export interface OverrideFieldState<T> {
  overridden: boolean
  value: T
}

export interface RuntimeSettingsFormState {
  queryMaxResultRows: OverrideFieldState<number>
  queryMaxResultBytes: OverrideFieldState<number>
  exportsSyncMaxBytes: OverrideFieldState<number>
  exportsBackgroundMaxBytes: OverrideFieldState<number>
  schemaSnapshotFreshnessSeconds: OverrideFieldState<number>
  fileRevisionsEnabled: OverrideFieldState<boolean>
  fileRevisionsKeepLatest: OverrideFieldState<number>
}

function fieldState<T>(override: T | null, effective: T): OverrideFieldState<T> {
  return { overridden: override !== null, value: override ?? effective }
}

export function runtimeSettingsFormState(
  settings: OrganizationRuntimeSettings,
): RuntimeSettingsFormState {
  return {
    queryMaxResultRows: fieldState(
      settings.overrides.query_max_result_rows,
      settings.effective.query_max_result_rows,
    ),
    queryMaxResultBytes: fieldState(
      settings.overrides.query_max_result_bytes,
      settings.effective.query_max_result_bytes,
    ),
    exportsSyncMaxBytes: fieldState(
      settings.overrides.exports_sync_max_bytes,
      settings.effective.exports_sync_max_bytes,
    ),
    exportsBackgroundMaxBytes: fieldState(
      settings.overrides.exports_background_max_bytes,
      settings.effective.exports_background_max_bytes,
    ),
    schemaSnapshotFreshnessSeconds: fieldState(
      settings.overrides.schema_snapshot_freshness_seconds,
      settings.effective.schema_snapshot_freshness_seconds,
    ),
    fileRevisionsEnabled: fieldState(
      settings.overrides.file_revisions_enabled,
      settings.effective.file_revisions_enabled,
    ),
    fileRevisionsKeepLatest: fieldState(
      settings.overrides.file_revisions_keep_latest,
      settings.effective.file_revisions_keep_latest,
    ),
  }
}

/**
 * Diffs the form against the last-loaded overrides and builds a partial PATCH body.
 * A field is omitted (no change), set to `null` (reset to inherit), or set to a value
 * (override), matching the backend's set-vs-omitted-vs-null PATCH semantics.
 */
export function buildRuntimeSettingsPatch(
  form: RuntimeSettingsFormState,
  original: OrganizationRuntimeOverrideValues,
): OrganizationRuntimeSettingsPatch {
  const patch: OrganizationRuntimeSettingsPatch = {}

  if (form.queryMaxResultRows.overridden) {
    if (original.query_max_result_rows !== form.queryMaxResultRows.value) {
      patch.query_max_result_rows = form.queryMaxResultRows.value
    }
  } else if (original.query_max_result_rows !== null) {
    patch.query_max_result_rows = null
  }

  if (form.queryMaxResultBytes.overridden) {
    if (original.query_max_result_bytes !== form.queryMaxResultBytes.value) {
      patch.query_max_result_bytes = form.queryMaxResultBytes.value
    }
  } else if (original.query_max_result_bytes !== null) {
    patch.query_max_result_bytes = null
  }

  if (form.exportsSyncMaxBytes.overridden) {
    if (original.exports_sync_max_bytes !== form.exportsSyncMaxBytes.value) {
      patch.exports_sync_max_bytes = form.exportsSyncMaxBytes.value
    }
  } else if (original.exports_sync_max_bytes !== null) {
    patch.exports_sync_max_bytes = null
  }

  if (form.exportsBackgroundMaxBytes.overridden) {
    if (original.exports_background_max_bytes !== form.exportsBackgroundMaxBytes.value) {
      patch.exports_background_max_bytes = form.exportsBackgroundMaxBytes.value
    }
  } else if (original.exports_background_max_bytes !== null) {
    patch.exports_background_max_bytes = null
  }

  if (form.schemaSnapshotFreshnessSeconds.overridden) {
    if (original.schema_snapshot_freshness_seconds !== form.schemaSnapshotFreshnessSeconds.value) {
      patch.schema_snapshot_freshness_seconds = form.schemaSnapshotFreshnessSeconds.value
    }
  } else if (original.schema_snapshot_freshness_seconds !== null) {
    patch.schema_snapshot_freshness_seconds = null
  }

  if (form.fileRevisionsEnabled.overridden) {
    if (original.file_revisions_enabled !== form.fileRevisionsEnabled.value) {
      patch.file_revisions_enabled = form.fileRevisionsEnabled.value
    }
  } else if (original.file_revisions_enabled !== null) {
    patch.file_revisions_enabled = null
  }

  if (form.fileRevisionsKeepLatest.overridden) {
    if (original.file_revisions_keep_latest !== form.fileRevisionsKeepLatest.value) {
      patch.file_revisions_keep_latest = form.fileRevisionsKeepLatest.value
    }
  } else if (original.file_revisions_keep_latest !== null) {
    patch.file_revisions_keep_latest = null
  }

  return patch
}

export function hasRuntimeSettingsChanges(
  form: RuntimeSettingsFormState,
  original: OrganizationRuntimeOverrideValues,
): boolean {
  return Object.keys(buildRuntimeSettingsPatch(form, original)).length > 0
}
