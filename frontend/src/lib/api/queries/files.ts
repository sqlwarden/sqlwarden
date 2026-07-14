import { queryOptions } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type { WorkspaceFileBrowserResult, WorkspaceFilesResponse } from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

export function orgWorkspacePrivateFilesQueryOptions(
  slug: string,
  workspaceId: string | number,
  parentId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePrivateFiles(slug, workspaceId, parentId),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(
          `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/private`,
          {
            query: { parent_id: parentId ?? undefined },
          },
        )
        .then((res) => res.files),
  })
}

export function orgWorkspaceSharedFilesQueryOptions(
  slug: string,
  workspaceId: string | number,
  parentId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceSharedFiles(slug, workspaceId, parentId),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(
          `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/shared`,
          {
            query: { parent_id: parentId ?? undefined },
          },
        )
        .then((res) => res.files),
  })
}

export function orgWorkspacePrivateFileBrowserQueryOptions(
  slug: string,
  workspaceId: string | number,
  fileId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePrivateFileBrowser(slug, workspaceId, fileId),
    queryFn: () =>
      api.get<WorkspaceFileBrowserResult>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/private/browser`,
        {
          query: { file_id: fileId ?? undefined },
        },
      ),
  })
}

export function orgWorkspaceSharedFileBrowserQueryOptions(
  slug: string,
  workspaceId: string | number,
  fileId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceSharedFileBrowser(slug, workspaceId, fileId),
    queryFn: () =>
      api.get<WorkspaceFileBrowserResult>(
        `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/shared/browser`,
        {
          query: { file_id: fileId ?? undefined },
        },
      ),
  })
}

export function orgWorkspacePrivateRecentFilesQueryOptions(
  slug: string,
  workspaceId: string | number,
  limit?: number,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspacePrivateRecentFiles(slug, workspaceId, limit),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(
          `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/private/recent`,
          {
            query: { limit },
          },
        )
        .then((res) => res.files),
  })
}

export function orgWorkspaceSharedRecentFilesQueryOptions(
  slug: string,
  workspaceId: string | number,
  limit?: number,
) {
  return queryOptions({
    queryKey: queryKeys.orgWorkspaceSharedRecentFiles(slug, workspaceId, limit),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(
          `/api/v1/orgs/${slug}/workspaces/${workspaceId}/files/shared/recent`,
          {
            query: { limit },
          },
        )
        .then((res) => res.files),
  })
}

export function myWorkspacePrivateFilesQueryOptions(
  workspaceId: string | number,
  parentId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.myWorkspacePrivateFiles(workspaceId, parentId),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(`/api/v1/me/workspaces/${workspaceId}/files/private`, {
          query: { parent_id: parentId ?? undefined },
        })
        .then((res) => res.files),
  })
}

export function myWorkspacePrivateFileBrowserQueryOptions(
  workspaceId: string | number,
  fileId?: string | number | null,
) {
  return queryOptions({
    queryKey: queryKeys.myWorkspacePrivateFileBrowser(workspaceId, fileId),
    queryFn: () =>
      api.get<WorkspaceFileBrowserResult>(
        `/api/v1/me/workspaces/${workspaceId}/files/private/browser`,
        {
          query: { file_id: fileId ?? undefined },
        },
      ),
  })
}

export function myWorkspacePrivateRecentFilesQueryOptions(
  workspaceId: string | number,
  limit?: number,
) {
  return queryOptions({
    queryKey: queryKeys.myWorkspacePrivateRecentFiles(workspaceId, limit),
    queryFn: () =>
      api
        .get<WorkspaceFilesResponse>(`/api/v1/me/workspaces/${workspaceId}/files/private/recent`, {
          query: { limit },
        })
        .then((res) => res.files),
  })
}
