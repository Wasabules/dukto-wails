package dev.wasabules.dukto.eff

import android.graphics.Bitmap
import android.graphics.Color
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel

/**
 * Encodes [text] as a QR-code [Bitmap] suitable for display in a
 * Compose Image. Used by the pairing dialog to render the 5-word
 * passphrase as a scannable variant; the Wails side uses
 * skip2/go-qrcode but produces the same payload (the raw passphrase
 * string), so the two stacks interoperate.
 */
fun encodeQr(text: String, sizePx: Int = 480): Bitmap {
    val hints = mapOf(
        EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
        EncodeHintType.MARGIN to 1,
    )
    val matrix = QRCodeWriter().encode(text, BarcodeFormat.QR_CODE, sizePx, sizePx, hints)
    val bmp = Bitmap.createBitmap(sizePx, sizePx, Bitmap.Config.ARGB_8888)
    for (y in 0 until sizePx) {
        for (x in 0 until sizePx) {
            bmp.setPixel(x, y, if (matrix.get(x, y)) Color.BLACK else Color.WHITE)
        }
    }
    return bmp
}
