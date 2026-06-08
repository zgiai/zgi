import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { useT } from '@/i18n';

interface RunWarningDialogProps {
  open: boolean;
  dontWarnAgain: boolean;
  onOpenChange: (open: boolean) => void;
  onDontWarnAgainChange: (value: boolean) => void;
  onViewErrors: () => void;
  onContinue: () => void;
}

export function RunWarningDialog({
  open,
  dontWarnAgain,
  onOpenChange,
  onDontWarnAgainChange,
  onViewErrors,
  onContinue,
}: RunWarningDialogProps) {
  const t = useT();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[440px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-black tracking-tight flex items-center gap-3">
            <div className="h-8 w-8 bg-amber-100 text-amber-500 flex items-center justify-center rounded-lg">
              <span className="text-lg font-black">!</span>
            </div>
            {t('agents.workflow.runErrorsDialog.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6">
          <div className="bg-amber-50/50 p-4 rounded-2xl border border-amber-100 text-sm font-medium leading-relaxed text-neutral-600">
            {t('agents.workflow.runErrorsDialog.description')}
          </div>

          <div
            className="flex items-center gap-3 px-1 group cursor-pointer"
            onClick={() => onDontWarnAgainChange(!dontWarnAgain)}
          >
            <Checkbox
              id="wf-chat-warn-hide"
              checked={dontWarnAgain}
              onCheckedChange={value => onDontWarnAgainChange(Boolean(value))}
              className="w-5 h-5"
            />
            <Label
              htmlFor="wf-chat-warn-hide"
              className="text-sm font-bold text-neutral-500 group-hover:text-primary transition-colors cursor-pointer"
            >
              {t('agents.workflow.runErrorsDialog.dontShowAgain')}
            </Label>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
          <Button variant="ghost" className="font-semibold" onClick={onViewErrors}>
            {t('agents.workflow.runErrorsDialog.viewErrors')}
          </Button>
          <Button size="lg" className="px-10 font-bold shadow-sm" onClick={onContinue}>
            {t('agents.workflow.runErrorsDialog.continueRun')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
