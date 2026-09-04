import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Browser, Events } from "@wailsio/runtime";
import {
  CheckForUpdates,
  GetAppInfo,
  RestartToUpdate,
  SetBetaChannel,
} from "../../bindings/cmdex/updateservice";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { MainLogo } from "../assets/images/main-logo";
import type { AppInfo } from "../types";
import { formatRelativeTime } from "../utils/relativeTime";

const REPO_URL = "https://github.com/loco1842/cmdex";
const LICENSE_URL = `${REPO_URL}/blob/main/LICENSE`;

function releaseNotesUrl(version: string): string {
  if (!version || version === "dev") return `${REPO_URL}/releases`;
  return `${REPO_URL}/releases/tag/v${version}`;
}

type UpdateStatus =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "downloading"; version: string; progress: number | null }
  | { kind: "verifying" }
  | { kind: "installing" }
  | { kind: "ready"; version: string }
  | { kind: "upToDate" }
  | { kind: "error"; message: string };

interface WailsEvent<T = unknown> {
  name: string;
  data: T;
  sender: string;
}

interface ReleasePayload {
  version?: string;
}

interface ProgressPayload {
  written?: number;
  total?: number;
}

interface ErrorPayload {
  stage?: string;
  message?: string;
}

function statusFromState(state: string, pendingVersion: string): UpdateStatus {
  switch (state) {
    case "checking":
      return { kind: "checking" };
    case "available":
    case "downloading":
      return { kind: "downloading", version: pendingVersion, progress: null };
    case "verifying":
      return { kind: "verifying" };
    case "installing":
      return { kind: "installing" };
    case "ready":
      return { kind: "ready", version: pendingVersion };
    case "up-to-date":
      return { kind: "upToDate" };
    case "error":
      return { kind: "error", message: "" };
    default:
      return { kind: "idle" };
  }
}

interface AboutDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export default function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
  const { t } = useTranslation();
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [status, setStatus] = useState<UpdateStatus>({ kind: "idle" });
  const [betaBusy, setBetaBusy] = useState(false);
  const [restartBusy, setRestartBusy] = useState(false);
  // Tracks whether a live check/download flow is in progress so the snapshot
  // refresh below doesn't clobber event-driven status. Written only in event
  // handlers and effects, never during render.
  const flowActiveRef = useRef(false);

  const refresh = useCallback(async (seedStatus: boolean) => {
    try {
      const snapshot = await GetAppInfo();
      setInfo(snapshot);
      // Only seed from the snapshot when no live flow owns the status —
      // events own it once a check starts. Terminal event handlers pass
      // false so the fresh snapshot can't clobber the just-settled state
      // (the mock resolves GetAppInfo with a stale pre-flow state).
      if (seedStatus && !flowActiveRef.current) {
        setStatus(statusFromState(snapshot.state, snapshot.pendingVersion));
      }
    } catch {
      // Dev/mock builds without the binding keep the dialog usable.
    }
  }, []);

  /* eslint-disable react-hooks/set-state-in-effect -- dialog-open snapshot load + updater event subscriptions (external system) */
  useEffect(() => {
    if (!open) return;
    flowActiveRef.current = false;
    void refresh(true);
    const checkStarted = () => {
      flowActiveRef.current = true;
      setStatus({ kind: "checking" });
    };
    const flowEnded = (next: UpdateStatus) => {
      flowActiveRef.current = false;
      setStatus(next);
    };
    const cleanups = [
      Events.On("wails:updater:check-started", checkStarted),
      Events.On("wails:updater:update-available", (e: WailsEvent<ReleasePayload>) => {
        flowActiveRef.current = true;
        setStatus({ kind: "downloading", version: e?.data?.version ?? "", progress: null });
      }),
      Events.On("wails:updater:download-progress", (e: WailsEvent<ProgressPayload>) => {
        const { written = 0, total = 0 } = e?.data ?? {};
        setStatus((prev) => {
          const version = prev.kind === "downloading" ? prev.version : "";
          return {
            kind: "downloading",
            version,
            progress: total > 0 ? Math.min(100, Math.round((written / total) * 100)) : null,
          };
        });
      }),
      Events.On("wails:updater:verifying", () => setStatus({ kind: "verifying" })),
      Events.On("wails:updater:installing", () => setStatus({ kind: "installing" })),
      Events.On("wails:updater:update-ready", (e: WailsEvent<ReleasePayload>) =>
        flowEnded({ kind: "ready", version: e?.data?.version ?? "" }),
      ),
      Events.On("wails:updater:no-update", () => {
        flowEnded({ kind: "upToDate" });
        void refresh(false);
      }),
      Events.On("wails:updater:error", (e: WailsEvent<ErrorPayload>) => {
        const message = e?.data?.message || "unknown error";
        flowEnded({ kind: "error", message });
      }),
    ];
    return () => {
      cleanups.forEach((cleanup) => cleanup());
    };
  }, [open, refresh]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const handleCheck = useCallback(async () => {
    setStatus({ kind: "checking" });
    try {
      await CheckForUpdates();
    } catch (err) {
      setStatus({ kind: "error", message: String(err) });
    }
  }, []);

  const handleBetaChange = useCallback(
    async (enabled: boolean) => {
      setBetaBusy(true);
      try {
        await SetBetaChannel(enabled);
        setInfo((prev) => (prev ? { ...prev, betaChannel: enabled } : prev));
        // An enabled beta channel usually means the user wants the RC now.
        if (enabled) await handleCheck();
      } catch {
        // Switch stays put on failure (info not updated).
      } finally {
        setBetaBusy(false);
      }
    },
    [handleCheck],
  );

  const handleRestart = useCallback(async () => {
    setRestartBusy(true);
    try {
      await RestartToUpdate();
    } catch {
      setRestartBusy(false);
    }
  }, []);

  const openExternal = useCallback((url: string) => {
    try {
      Browser.OpenURL(url);
    } catch {
      window.open(url, "_blank", "noopener");
    }
  }, []);

  const busy = status.kind === "checking" || status.kind === "downloading" || status.kind === "verifying" || status.kind === "installing";
  const version = info?.version ?? "dev";
  const updatesEnabled = info?.updatesEnabled ?? false;
  const lastChecked = info?.lastCheck ? formatRelativeTime(info.lastCheck) : "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" data-testid="about-dialog">
        <div className="flex flex-col items-center text-center pt-2">
          <MainLogo width={64} height={64} />
          <h2 className="mt-4 text-lg font-semibold">{t("about.title")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("about.version", { version, arch: info?.arch ?? "" })}{" "}
            <button
              type="button"
              className="text-primary underline underline-offset-2 hover:no-underline"
              onClick={() => openExternal(releaseNotesUrl(version))}
            >
              {t("about.releaseNotes")}
            </button>
          </p>

          <div className="mt-2 min-h-6 text-sm" data-testid="about-status" aria-live="polite">
            {!updatesEnabled && <span className="text-muted-foreground">{t("about.devVersion")}</span>}
            {updatesEnabled && status.kind === "idle" && (
              <span className="text-muted-foreground">
                {lastChecked ? t("about.lastCheckedOnly", { when: lastChecked }) : t("about.notCheckedYet")}
              </span>
            )}
            {updatesEnabled && status.kind === "checking" && <span>{t("about.checking")}</span>}
            {updatesEnabled && status.kind === "downloading" && (
              <span>
                {t("about.downloading")}
                {status.progress !== null && ` ${status.progress}%`}
              </span>
            )}
            {updatesEnabled && status.kind === "verifying" && <span>{t("about.verifying")}</span>}
            {updatesEnabled && status.kind === "installing" && <span>{t("about.installing")}</span>}
            {updatesEnabled && status.kind === "ready" && (
              <span>{t("about.ready", { version: status.version })}</span>
            )}
            {updatesEnabled && status.kind === "upToDate" && (
              <span>
                {t("about.upToDate")}
                {lastChecked && ` ${t("about.lastChecked", { when: lastChecked })}`}
              </span>
            )}
            {updatesEnabled && status.kind === "error" && (
              <span className="text-destructive">{t("about.checkFailed", { message: status.message })}</span>
            )}
          </div>

          {status.kind === "downloading" && status.progress !== null && (
            <div className="mt-2 h-1.5 w-48 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-primary transition-[width]"
                style={{ width: `${status.progress}%` }}
              />
            </div>
          )}

          <div className="mt-3 flex gap-2">
            {status.kind === "ready" && updatesEnabled ? (
              <Button type="button" size="sm" disabled={restartBusy} onClick={handleRestart}>
                {t("about.restart")}
              </Button>
            ) : (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!updatesEnabled || busy}
                onClick={handleCheck}
              >
                {busy ? t("about.checking") : t("about.checkForUpdates")}
              </Button>
            )}
          </div>

          {updatesEnabled && (
            <div className="mt-5 w-full border-t border-border pt-4 text-left">
              <p className="text-center text-sm">{t("about.betaHeading")}</p>
              <div className="mt-2 flex items-center justify-between gap-4">
                <div className="space-y-0.5">
                  <Label htmlFor="beta-channel-toggle">{t("about.betaToggle")}</Label>
                  <p className="text-[11px] text-muted-foreground">{t("about.betaHint")}</p>
                </div>
                <Switch
                  id="beta-channel-toggle"
                  checked={info?.betaChannel ?? false}
                  disabled={betaBusy}
                  onCheckedChange={handleBetaChange}
                />
              </div>
            </div>
          )}

          <div className="mt-4 flex flex-col items-center gap-1.5 text-sm">
            <button
              type="button"
              className="text-primary underline underline-offset-2 hover:no-underline"
              onClick={() => openExternal(LICENSE_URL)}
            >
              {t("about.license")}
            </button>
            <button
              type="button"
              className="text-primary underline underline-offset-2 hover:no-underline"
              onClick={() => openExternal(REPO_URL)}
            >
              {t("about.viewOnGitHub")}
            </button>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" onClick={() => onOpenChange(false)}>
            {t("about.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
