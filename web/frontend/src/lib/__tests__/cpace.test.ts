import { ristretto255 } from "@noble/curves/ed25519.js";
import { sha512 } from "@noble/hashes/sha2.js";
import { bytesToHex, hexToBytes } from "@noble/hashes/utils.js";
import {
  CPaceError,
  calculateGenerator,
  cpaceIsk,
  cpaceShare,
  generatorString,
  lvCat,
  prependLen,
  sampleScalar,
  scalarFromBytesLE,
  scalarMultVfy,
  transcriptIr,
} from "../cpace";

// draft-irtf-cfrg-cpace-21, appendix B.3 (CPACE-RISTR255-SHA512).
const h = (hex: string) => hexToBytes(hex.replace(/\s+/g, ""));
const utf8 = (s: string) => new TextEncoder().encode(s);

const PRS = utf8("Password");
const CI = h("0b415f696e69746961746f720b425f726573706f6e646572");
const SID = h("7e4b4791d6a8ef019b936c79fb7f2c57");
const ADA = utf8("ADa");
const ADB = utf8("ADb");
const YA_SCALAR = h("da3d23700a9e5699258aef94dc060dfda5ebb61f02a5ea77fad53f4ff0976d08");
const YB_SCALAR = h("d2316b454718c35362d83d69df6320f38578ed5984651435e2949762d900b80d");
const YA = h("d6bac480f2c386c394efc7c47adb9925dcd2630b64f240c50f8d0eec482b9157");
const YB = h("3ea7e0b19560d7c0b0f5734f63b955286dfa8232b5ebe63324e2d9e7433f7258");
const K = h("80b69a8a76457ab6a4d7f887a4bf6b55a2f80ac19c333f917a05fc9887c8b40f");

describe("CPace encoding helpers (draft appendix A)", () => {
  it("prepends LEB128 lengths", () => {
    expect(bytesToHex(prependLen(new Uint8Array()))).toBe("00");
    expect(bytesToHex(prependLen(utf8("1234")))).toBe("0431323334");
    const long = Uint8Array.from({ length: 128 }, (_, i) => i);
    expect(bytesToHex(prependLen(long).slice(0, 3))).toBe("800100");
  });

  it("concatenates length-value fields", () => {
    expect(bytesToHex(lvCat(utf8("1234"), utf8("5"), new Uint8Array(), utf8("678")))).toBe(
      "043132333401350003363738",
    );
  });

  it("builds the initiator-responder transcript", () => {
    expect(bytesToHex(transcriptIr(utf8("123"), utf8("PartyA"), utf8("234"), utf8("PartyB")))).toBe(
      "03313233065061727479410332333406506172747942",
    );
  });
});

describe("CPace on ristretto255 (draft appendix B.3)", () => {
  it("calculates the generator string, its hash and the generator", () => {
    const genStr = generatorString(PRS, CI, SID);

    expect(genStr.length).toBe(170);
    expect(bytesToHex(genStr)).toBe(
      "11435061636552697374726574746f3235350850617373776f726464" +
        "00".repeat(100) +
        "180b415f696e69746961746f720b425f726573706f6e646572107e4b4791d6a8ef019b936c79fb7f2c57",
    );
    expect(bytesToHex(sha512(genStr))).toBe(
      "da6d3ddc8802fca9058755ffd3ebde08a9c2c74945901a258482a288b6663af06bf645c93cd1c51512307199c80e84908916d983b34af77205f90851a657ee27",
    );
    expect(bytesToHex(calculateGenerator(PRS, CI, SID).toBytes())).toBe(
      "222b6b195fe84b1652badb6f6a3ae3d24341e7306967f0b8115b40d5698c7e56",
    );
  });

  it("produces both parties' shares", () => {
    const g = calculateGenerator(PRS, CI, SID);

    expect(cpaceShare(g, scalarFromBytesLE(YA_SCALAR))).toEqual(YA);
    expect(cpaceShare(g, scalarFromBytesLE(YB_SCALAR))).toEqual(YB);
  });

  it("derives the same secret point K on both sides", () => {
    expect(scalarMultVfy(scalarFromBytesLE(YA_SCALAR), YB)).toEqual(K);
    expect(scalarMultVfy(scalarFromBytesLE(YB_SCALAR), YA)).toEqual(K);
  });

  it("derives the initiator-responder ISK", () => {
    expect(bytesToHex(cpaceIsk(SID, K, YA, ADA, YB, ADB))).toBe(
      "b69effbf61b51d56401c0f65601abe428de8206feaaf0e32198896dcae7b35cd2b38950a39dfd5d4a79164614c2984f7daa460b588c1e80c3fa2068af7900447",
    );
  });

  it("multiplies a valid encoded point (B.3.10)", () => {
    const s = scalarFromBytesLE(
      h("7cd0e075fa7955ba52c02759a6c90dbbfc10e6d40aea8d283e407d88cf538a05"),
    );
    const x = h("2c3c6b8c4f3800e7aef6864025b4ed79bd599117e427c41bd47d93d654b4a51c");

    expect(bytesToHex(scalarMultVfy(s, x))).toBe(
      "7c13645fe790a468f62c39beb7388e541d8405d1ade69d1778c5fe3e7f6b600e",
    );
  });

  it("rejects an invalid encoding and the neutral element (B.3.11)", () => {
    const s = scalarFromBytesLE(
      h("7cd0e075fa7955ba52c02759a6c90dbbfc10e6d40aea8d283e407d88cf538a05"),
    );
    const invalid = h("2b3c6b8c4f3800e7aef6864025b4ed79bd599117e427c41bd47d93d654b4a51c");

    expect(() => scalarMultVfy(s, invalid)).toThrow(CPaceError);
    expect(() => scalarMultVfy(s, new Uint8Array(32))).toThrow(CPaceError);
  });
});

describe("sampleScalar", () => {
  it("returns non-zero scalars below 2^252", () => {
    for (let i = 0; i < 50; i++) {
      const scalar = sampleScalar();
      expect(scalar > 0n).toBe(true);
      expect(scalar < 1n << 252n).toBe(true);
      expect(scalar < ristretto255.Point.Fn.ORDER).toBe(true);
    }
  });
});
