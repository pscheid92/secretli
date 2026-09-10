import { useEffect, useState } from "react";

interface QRCodeProps {
  url: string;
}

export default function QRCode({ url }: QRCodeProps) {
  const [dataUrl, setDataUrl] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // The QR library is only needed once a link exists; keep it out of the
    // initial bundle.
    import("qrcode")
      .then((QRCodeLib) => QRCodeLib.toDataURL(url, { errorCorrectionLevel: "M", margin: 2 }))
      .then((result) => {
        if (!cancelled) setDataUrl(result);
      })
      .catch(console.error);
    return () => {
      cancelled = true;
    };
  }, [url]);

  if (!dataUrl) return null;

  return (
    <div className="inline-block rounded-lg bg-white p-2">
      <img src={dataUrl} alt="QR code for share link" width={160} height={160} />
    </div>
  );
}
