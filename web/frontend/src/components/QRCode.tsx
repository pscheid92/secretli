import { useEffect, useState } from "react";

interface QRCodeProps {
  url: string;
  /** Quiet zone around the code, in modules. The QR spec asks for 4. */
  margin?: number;
  className?: string;
}

export default function QRCode({ url, margin = 2, className = "h-40 w-40" }: QRCodeProps) {
  const [dataUrl, setDataUrl] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // The QR library is only needed once a link exists; keep it out of the
    // initial bundle. SVG stays sharp at whatever size the code is shown, and
    // is always dark on white: some detectors cannot read inverted codes.
    import("qrcode")
      .then((QRCodeLib) =>
        QRCodeLib.toString(url, { type: "svg", errorCorrectionLevel: "M", margin }),
      )
      .then((svg) => {
        if (!cancelled) setDataUrl(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`);
      })
      .catch(console.error);
    return () => {
      cancelled = true;
    };
  }, [url, margin]);

  if (!dataUrl) return null;

  return <img src={dataUrl} alt="QR code for share link" className={className} />;
}
