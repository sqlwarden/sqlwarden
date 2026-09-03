import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { AppearancePanel } from './AppearancePanel'

export function AppearanceDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Appearance</DialogTitle>
        </DialogHeader>
        <AppearancePanel />
      </DialogContent>
    </Dialog>
  )
}
