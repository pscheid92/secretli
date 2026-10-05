import { useEffect, useRef, useState } from "react";
import {
  type CameraFailure,
  cameraFailure,
  createDecoder,
  openCamera,
  type QRDecoder,
  scanFrames,
  stopCamera,
} from "../../lib/qrScanner";
import { parseShareLink } from "../../lib/shareLink";
import Spinner from "../Spinner";
import Button from "../ui/Button";

/** How long to scan before suggesting ways to get a better read. */
export const SLOW_SCAN_MS = 20_000;

const FAILURE_MESSAGES = {
  "no-camera": "No camera found. Paste or type the link instead.",
  "camera-failed":
    "The camera couldn't be started. Close other apps using it and try again, or paste the link instead.",
  blocked: "Camera access is blocked. Allow it in the site settings, or paste the link instead.",
  unavailable: "The QR scanner couldn't start. Paste or type the link instead.",
} satisfies Record<CameraFailure | "unavailable", string>;

type Failure = keyof typeof FAILURE_MESSAGES;

interface QRScannerProps {
  /** Called once, with the fragment of a valid share link for this site. */
  onScan: (fragment: string) => void;
  onCancel: () => void;
}

/**
 * Reads a share link's QR code with the camera. Frames are decoded in this
 * browser and never leave it; the camera is released on every exit.
 */
export default function QRScanner({ onScan, onCancel }: QRScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const onScanRef = useRef(onScan);
  const [running, setRunning] = useState(true);
  const [ready, setReady] = useState(false);
  const [done, setDone] = useState(false);
  const [failure, setFailure] = useState<Failure | null>(null);
  const [hint, setHint] = useState<string | null>(null);
  const [slow, setSlow] = useState(false);
  const [mirrored, setMirrored] = useState(true);

  useEffect(() => {
    onScanRef.current = onScan;
  });

  useEffect(() => {
    if (!running) return;
    const video = videoRef.current;
    if (!video) return;

    let cancelled = false;
    let stream: MediaStream | null = null;
    let stopFrames = () => {};
    let slowTimer: ReturnType<typeof setTimeout> | undefined;

    function release() {
      stopFrames();
      clearTimeout(slowTimer);
      if (stream) stopCamera(stream);
      stream = null;
    }

    function fail(reason: Failure) {
      release();
      if (cancelled) return;
      setFailure(reason);
      setRunning(false);
    }

    async function start(video: HTMLVideoElement) {
      try {
        stream = await openCamera();
      } catch (err) {
        fail(cameraFailure(err));
        return;
      }
      if (cancelled) {
        release();
        return;
      }

      // A camera facing the user is mirrored so aiming feels natural; the
      // decoder always sees the unmirrored frames.
      setMirrored(stream.getVideoTracks()[0]?.getSettings().facingMode !== "environment");

      let decoder: QRDecoder;
      try {
        decoder = await createDecoder();
        video.srcObject = stream;
        await video.play();
      } catch {
        fail("unavailable");
        return;
      }
      if (cancelled) {
        release();
        return;
      }

      setReady(true);
      slowTimer = setTimeout(() => setSlow(true), SLOW_SCAN_MS);
      stopFrames = scanFrames(video, decoder, (texts) => {
        for (const text of texts) {
          const link = parseShareLink(text, window.location.origin);
          if (link.kind === "share") {
            release();
            setDone(true);
            onScanRef.current(link.fragment);
            return true;
          }
          setHint(
            link.kind === "other-host"
              ? `This code is for ${link.host}, not this site.`
              : "That code isn't a Secretli link.",
          );
        }
        return false;
      });
    }

    start(video);
    return () => {
      cancelled = true;
      release();
      video.srcObject = null;
    };
  }, [running]);

  useEffect(() => {
    function handleVisibilityChange() {
      if (document.hidden) setRunning(false);
    }
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => document.removeEventListener("visibilitychange", handleVisibilityChange);
  }, []);

  function restart() {
    setFailure(null);
    setHint(null);
    setSlow(false);
    setReady(false);
    setRunning(true);
  }

  let message: string;
  if (failure) message = FAILURE_MESSAGES[failure];
  else if (done) message = "Opening the secret…";
  else if (!running) message = "Scanning paused while this tab was in the background.";
  else if (!ready) message = "Starting the camera…";
  else if (hint) message = hint;
  else message = "Point the camera at the QR code on the other screen.";

  return (
    <div className="space-y-6">
      {failure === null && (
        <div className="relative aspect-video overflow-hidden rounded-[20px] bg-[#0a0a0b] ring-1 ring-line">
          <video
            ref={videoRef}
            muted
            playsInline
            aria-label="Camera preview"
            className={`h-full w-full object-cover ${mirrored ? "-scale-x-100" : ""}`}
          />
          {running && !ready && (
            <div className="absolute inset-0 flex items-center justify-center text-[#8a8a93]">
              <Spinner size="lg" />
            </div>
          )}
        </div>
      )}
      <div role="status" className="space-y-1.5">
        <p className="text-body text-muted">{message}</p>
        {slow && running && !done && (
          <p className="text-[13px] text-faint">
            Having trouble? Turn up the other screen's brightness, hold it 30 to 50 cm from the
            camera, and tilt it to avoid glare.
          </p>
        )}
      </div>
      <div className="flex gap-1">
        {!running && !done && (
          <Button size="lg" onClick={restart}>
            {failure ? "Try again" : "Resume"}
          </Button>
        )}
        <Button variant="quiet" size="lg" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
