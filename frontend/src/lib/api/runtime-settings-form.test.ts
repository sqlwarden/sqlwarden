import { describe, expect, it } from 'vitest'
import type {
  OrganizationRuntimeOverrideValues,
  OrganizationRuntimeSettings,
} from '#/lib/api/types'
import {
  buildRuntimeSettingsPatch,
  hasRuntimeSettingsChanges,
  runtimeSettingsFormState,
} from './runtime-settings-form'

function settingsFixture(
  overrides: Partial<OrganizationRuntimeOverrideValues> = {},
): OrganizationRuntimeSettings {
  return {
    overrides: {
      query_max_result_rows: null,
      query_max_result_bytes: null,
      exports_sync_max_bytes: null,
      exports_background_max_bytes: null,
      schema_snapshot_freshness_seconds: null,
      file_revisions_enabled: null,
      file_revisions_keep_latest: null,
      ...overrides,
    },
    effective: {
      query_max_result_rows: 1_000,
      query_max_result_bytes: 10_485_760,
      exports_sync_max_bytes: 52_428_800,
      exports_background_max_bytes: 0,
      schema_snapshot_freshness_seconds: 3_600,
      file_revisions_enabled: true,
      file_revisions_keep_latest: 10,
    },
    constraints: {
      query_max_result_rows_max: 5_000,
      query_max_result_bytes_max: 104_857_600,
      exports_sync_max_bytes_max: 209_715_200,
      exports_background_max_bytes_max: 0,
      schema_snapshot_freshness_seconds_min: 900,
      file_revisions_available: true,
      file_revisions_keep_latest_max: 50,
    },
  }
}

describe('runtimeSettingsFormState', () => {
  it('marks fields not overridden and seeds them with the effective value', () => {
    const form = runtimeSettingsFormState(settingsFixture())

    expect(form.queryMaxResultRows).toEqual({ overridden: false, value: 1_000 })
    expect(form.fileRevisionsEnabled).toEqual({ overridden: false, value: true })
  })

  it('marks fields overridden and seeds them with the override value', () => {
    const form = runtimeSettingsFormState(
      settingsFixture({ query_max_result_rows: 250, file_revisions_enabled: false }),
    )

    expect(form.queryMaxResultRows).toEqual({ overridden: true, value: 250 })
    expect(form.fileRevisionsEnabled).toEqual({ overridden: true, value: false })
  })
})

describe('buildRuntimeSettingsPatch', () => {
  it('omits fields that were not touched', () => {
    const settings = settingsFixture()
    const form = runtimeSettingsFormState(settings)

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({})
    expect(hasRuntimeSettingsChanges(form, settings.overrides)).toBe(false)
  })

  it('sends the value when a field is newly overridden', () => {
    const settings = settingsFixture()
    const form = runtimeSettingsFormState(settings)
    form.queryMaxResultRows = { overridden: true, value: 500 }

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({
      query_max_result_rows: 500,
    })
  })

  it('sends null when a previously overridden field is reset to inherit', () => {
    const settings = settingsFixture({ query_max_result_rows: 250 })
    const form = runtimeSettingsFormState(settings)
    form.queryMaxResultRows.overridden = false

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({
      query_max_result_rows: null,
    })
  })

  it('sends the new value when an already-overridden field changes', () => {
    const settings = settingsFixture({ query_max_result_rows: 250 })
    const form = runtimeSettingsFormState(settings)
    form.queryMaxResultRows.value = 300

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({
      query_max_result_rows: 300,
    })
  })

  it('omits an override that is toggled on then back off without changing', () => {
    const settings = settingsFixture()
    const form = runtimeSettingsFormState(settings)
    form.fileRevisionsKeepLatest.overridden = true
    form.fileRevisionsKeepLatest.overridden = false

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({})
  })

  it('diffs every field independently in a single patch', () => {
    const settings = settingsFixture({ exports_sync_max_bytes: 1_000 })
    const form = runtimeSettingsFormState(settings)
    form.exportsSyncMaxBytes.overridden = false // reset to inherit
    form.schemaSnapshotFreshnessSeconds = { overridden: true, value: 1_800 } // new override
    form.fileRevisionsKeepLatest = { overridden: true, value: 5 } // new override

    expect(buildRuntimeSettingsPatch(form, settings.overrides)).toEqual({
      exports_sync_max_bytes: null,
      schema_snapshot_freshness_seconds: 1_800,
      file_revisions_keep_latest: 5,
    })
  })
})
