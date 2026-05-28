import { HugeiconsIcon } from '@hugeicons/react'
import { ArrowExpandIcon, ArrowShrinkIcon, TableIcon } from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '#/components/ui/tabs'
import { useIde } from './useIdeStore'

export function ResultsArea() {
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'results' ? null : 'results')
  }

  return (
    <Tabs defaultValue="results" className="flex min-h-0 flex-1 flex-col gap-0">
      <div className="flex h-9 shrink-0 items-center justify-between border-b border-border bg-background px-1">
        <TabsList variant="line" className="h-full gap-0 rounded-none p-0">
          <TabsTrigger
            value="results"
            className="h-full rounded-none px-3 text-xs gap-1.5"
          >
            <HugeiconsIcon icon={TableIcon} size={13} strokeWidth={2} />
            Results
          </TabsTrigger>
          <TabsTrigger value="history" className="h-full rounded-none px-3 text-xs">
            History
          </TabsTrigger>
          <TabsTrigger value="explain" className="h-full rounded-none px-3 text-xs">
            Explain
          </TabsTrigger>
        </TabsList>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Toggle results maximize"
          onClick={toggleMaximize}
        >
          <HugeiconsIcon
            icon={maximizedPane === 'results' ? ArrowShrinkIcon : ArrowExpandIcon}
            size={14}
            strokeWidth={2}
          />
        </Button>
      </div>

      <TabsContent value="results" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <ResultsTable />
      </TabsContent>
      <TabsContent value="history" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <StubPane message="Query history coming soon." />
      </TabsContent>
      <TabsContent value="explain" className="min-h-0 flex-1 overflow-hidden m-0 p-0">
        <StubPane message="Execution plan coming soon." />
      </TabsContent>
    </Tabs>
  )
}

function ResultsTable() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <div className="flex min-h-0 flex-1 items-center justify-center p-8 text-center">
        <div className="flex flex-col gap-1.5">
          <div className="text-sm font-medium text-foreground">Run a query to see results</div>
          <div className="text-xs text-muted-foreground">Select a connection and press Run</div>
        </div>
      </div>
    </div>
  )
}

function StubPane({ message }: { message: string }) {
  return (
    <div className="flex h-full items-center justify-center bg-card">
      <p className="text-xs text-muted-foreground">{message}</p>
    </div>
  )
}
