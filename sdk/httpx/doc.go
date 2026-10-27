// Package httpx adalah satu-satunya implementasi amplop respons dan error HTTP ERPora
// (api-guidelines §4 dan §8) untuk platform maupun modul. Amplop sukses memakai kunci "data"
// (+ "page" untuk daftar), amplop error memakai "error" dengan requestId dan traceId.
package httpx
