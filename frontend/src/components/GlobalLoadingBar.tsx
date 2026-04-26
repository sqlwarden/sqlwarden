import { useIsFetching, useIsMutating } from '@tanstack/react-query'

export function GlobalLoadingBar() {
  const isFetching = useIsFetching()
  const isMutating = useIsMutating()
  const isLoading = isFetching + isMutating > 0

  return (
    <div
      className="pointer-events-none fixed inset-x-0 top-0 z-100 h-0.5 overflow-hidden"
      aria-hidden={!isLoading}
      data-loading={isLoading}
    >
      <div className="h-full w-full origin-left scale-x-0 bg-primary opacity-0 transition-[opacity,transform] duration-150 data-[loading=true]:scale-x-100 data-[loading=true]:opacity-100" data-loading={isLoading}>
        <div className="h-full w-1/2 animate-[global-loading-bar_1s_ease-in-out_infinite] bg-primary" />
      </div>
    </div>
  )
}
