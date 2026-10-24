#include "depthmeshgeometry.hpp"

#include <QFileInfo>
#include <QImage>
#include <QMetaObject>
#include <QVector3D>

#include <algorithm>
#include <array>
#include <cmath>
#include <cstddef>
#include <cstdint>
#include <limits>
#include <numbers>
#include <vector>

namespace {

constexpr int kColumns = 321;
constexpr int kRows = 181;
constexpr int kRefine = 4;
constexpr int kFineColumns = (kColumns - 1) * kRefine + 1;
constexpr int kFineRows = (kRows - 1) * kRefine + 1;
constexpr double kFarDistance = 3.3;
constexpr double kDepthSpan = 0.8;
constexpr double kNearScale = 0.8;
constexpr double kFarScale = 0.8;
constexpr double kFovDegrees = 42.0;
constexpr double kRefineThreshold = 0.08;
constexpr float kPivotDepth = 0.5f;

struct Vertex {
    float x, y, z, u, v;
};

QString localPath(const QUrl &url)
{
    const QString path = url.isLocalFile() ? url.toLocalFile()
        : url.scheme().isEmpty() ? url.path(QUrl::FullyDecoded) : QString();
    return QFileInfo(path).isAbsolute() ? path : QString();
}

struct ImagePlane {
    QImage image;
    int cropX = 0;
    int cropY = 0;
    int cropW = 0;
    int cropH = 0;
    bool wide = false;

    bool load(const QUrl &url, QImage::Format format, float outputAspect)
    {
        const QString path = localPath(url);
        if (path.isEmpty())
            return false;
        QImage loaded(path);
        if (loaded.isNull())
            return false;
        image = loaded.convertToFormat(format);
        if (image.isNull())
            return false;
        if (image.width() < 2 || image.height() < 2
            || !std::isfinite(outputAspect) || outputAspect <= 0.0f)
            return false;
        wide = image.format() == QImage::Format_Grayscale16;

        const float aspect = float(image.width()) / float(image.height());
        if (aspect > outputAspect) {
            cropH = image.height();
            cropW = int(std::lround(image.height() * outputAspect));
            cropX = (image.width() - cropW) / 2;
        } else {
            cropW = image.width();
            cropH = int(std::lround(image.width() / outputAspect));
            cropY = (image.height() - cropH) / 2;
        }
        cropW = std::clamp(cropW, 2, image.width());
        cropH = std::clamp(cropH, 2, image.height());
        return true;
    }

    float read(int x, int y) const
    {
        const uchar *row = image.constScanLine(y);
        if (wide)
            return reinterpret_cast<const quint16 *>(row)[x] / 65535.0f;
        return row[x] / 255.0f;
    }

    float at(float u, float v) const
    {
        const float fx = cropX + u * (cropW - 1);
        const float fy = cropY + v * (cropH - 1);
        const int x0 = std::clamp(int(std::floor(fx)), 0, image.width() - 1);
        const int y0 = std::clamp(int(std::floor(fy)), 0, image.height() - 1);
        const int x1 = std::min(x0 + 1, image.width() - 1);
        const int y1 = std::min(y0 + 1, image.height() - 1);
        const float tx = fx - x0;
        const float ty = fy - y0;
        const float top = read(x0, y0) * (1.0f - tx) + read(x1, y0) * tx;
        const float bottom = read(x0, y1) * (1.0f - tx) + read(x1, y1) * tx;
        return top * (1.0f - ty) + bottom * ty;
    }
};

float maskAt(const std::vector<float> &mask, float u, float v)
{
    const float fx = std::clamp(u * (kColumns - 1), 0.0f, float(kColumns - 1));
    const float fy = std::clamp(v * (kRows - 1), 0.0f, float(kRows - 1));
    const int x0 = int(std::floor(fx));
    const int y0 = int(std::floor(fy));
    const int x1 = std::min(x0 + 1, kColumns - 1);
    const int y1 = std::min(y0 + 1, kRows - 1);
    const float tx = fx - x0;
    const float ty = fy - y0;
    const float top = mask[y0 * kColumns + x0] * (1.0f - tx)
        + mask[y0 * kColumns + x1] * tx;
    const float bottom = mask[y1 * kColumns + x0] * (1.0f - tx)
        + mask[y1 * kColumns + x1] * tx;
    return top * (1.0f - ty) + bottom * ty;
}

void dilate(std::vector<float> &mask, int radius)
{
    std::vector<float> out(mask.size());
    for (int y = 0; y < kRows; ++y) {
        for (int x = 0; x < kColumns; ++x) {
            float value = 0.0f;
            for (int oy = -radius; oy <= radius; ++oy) {
                const int yy = std::clamp(y + oy, 0, kRows - 1);
                for (int ox = -radius; ox <= radius; ++ox) {
                    const int xx = std::clamp(x + ox, 0, kColumns - 1);
                    value = std::max(value, mask[yy * kColumns + xx]);
                }
            }
            out[y * kColumns + x] = value;
        }
    }
    mask.swap(out);
}

void blur(std::vector<float> &mask)
{
    std::vector<float> out(mask.size());
    for (int y = 0; y < kRows; ++y) {
        for (int x = 0; x < kColumns; ++x) {
            float sum = 0.0f;
            for (int oy = -1; oy <= 1; ++oy) {
                const int yy = std::clamp(y + oy, 0, kRows - 1);
                for (int ox = -1; ox <= 1; ++ox) {
                    const int xx = std::clamp(x + ox, 0, kColumns - 1);
                    sum += mask[yy * kColumns + xx];
                }
            }
            out[y * kColumns + x] = sum / 9.0f;
        }
    }
    mask.swap(out);
}

} // namespace

DepthMeshGeometry::DepthMeshGeometry(QQuick3DObject *parent)
    : QQuick3DGeometry(parent)
{
    setStride(sizeof(Vertex));
    setPrimitiveType(PrimitiveType::Triangles);
    addAttribute(Attribute::PositionSemantic, offsetof(Vertex, x), Attribute::F32Type);
    addAttribute(Attribute::TexCoord0Semantic, offsetof(Vertex, u), Attribute::F32Type);
}

void DepthMeshGeometry::setDepthPath(const QUrl &path)
{
    if (m_depthPath == path)
        return;
    m_depthPath = path;
    emit depthPathChanged();
    scheduleRebuild();
}

void DepthMeshGeometry::setMattePath(const QUrl &path)
{
    if (m_mattePath == path)
        return;
    m_mattePath = path;
    emit mattePathChanged();
    scheduleRebuild();
}

void DepthMeshGeometry::setImageZoom(float zoom)
{
    const float bounded = std::clamp(zoom, 1.0f, 1.5f);
    if (std::abs(m_imageZoom - bounded) < 1e-5f)
        return;
    m_imageZoom = bounded;
    emit imageZoomChanged();
    scheduleRebuild();
}

void DepthMeshGeometry::setOutputAspect(float aspect)
{
    const float bounded = std::clamp(aspect, 0.5f, 4.0f);
    if (std::abs(m_outputAspect - bounded) < 1e-5f)
        return;
    m_outputAspect = bounded;
    emit outputAspectChanged();
    scheduleRebuild();
}

void DepthMeshGeometry::setSourceAspect(float aspect)
{
    const float bounded = std::clamp(aspect, 0.01f, 100.0f);
    if (std::abs(m_sourceAspect - bounded) < 1e-5f)
        return;
    m_sourceAspect = bounded;
    emit sourceAspectChanged();
    scheduleRebuild();
}

void DepthMeshGeometry::scheduleRebuild()
{
    if (m_rebuildPending)
        return;
    m_rebuildPending = true;
    QMetaObject::invokeMethod(this, [this] {
        m_rebuildPending = false;
        rebuild();
    }, Qt::QueuedConnection);
}

void DepthMeshGeometry::setValid(bool valid)
{
    if (m_valid == valid)
        return;
    m_valid = valid;
    emit validChanged();
}

void DepthMeshGeometry::rebuild()
{
    setValid(false);
    setVertexData({});

    ImagePlane depth;
    if (!depth.load(m_depthPath, QImage::Format_Grayscale16, m_outputAspect))
        return;
    ImagePlane matte;
    const bool hasMatte = matte.load(m_mattePath, QImage::Format_Alpha8, m_outputAspect);

    std::vector<float> hole(kColumns * kRows, 0.0f);
    std::vector<float> filled(kColumns * kRows, kPivotDepth);
    if (hasMatte) {
        for (int y = 0; y < kRows; ++y) {
            for (int x = 0; x < kColumns; ++x) {
                const float u = float(x) / float(kColumns - 1);
                const float v = float(y) / float(kRows - 1);
                hole[y * kColumns + x] = matte.at(u, v) > 24.0f / 255.0f ? 1.0f : 0.0f;
            }
        }
        dilate(hole, 2);
        const std::vector<float> core = hole;
        blur(hole);

        std::vector<uint8_t> known(kColumns * kRows, 0);
        std::vector<float> background;
        background.reserve(kColumns * kRows);
        for (int y = 0; y < kRows; ++y) {
            for (int x = 0; x < kColumns; ++x) {
                const int i = y * kColumns + x;
                if (core[i] > 0.5f)
                    continue;
                const float u = float(x) / float(kColumns - 1);
                const float v = float(y) / float(kRows - 1);
                filled[i] = depth.at(u, v);
                known[i] = 1;
                background.push_back(filled[i]);
            }
        }
        float fallback = kPivotDepth;
        if (!background.empty()) {
            const auto mid = background.begin() + background.size() / 2;
            std::nth_element(background.begin(), mid, background.end());
            fallback = *mid;
        }

        std::vector<int> queue;
        queue.reserve(kColumns * kRows);
        const auto pushIfUnknown = [&](int i) {
            if (!known[i])
                queue.push_back(i);
        };
        for (int y = 0; y < kRows; ++y) {
            for (int x = 0; x < kColumns; ++x) {
                const int i = y * kColumns + x;
                if (known[i])
                    continue;
                if ((x > 0 && known[i - 1]) || (x + 1 < kColumns && known[i + 1])
                    || (y > 0 && known[i - kColumns]) || (y + 1 < kRows && known[i + kColumns]))
                    queue.push_back(i);
            }
        }
        for (size_t head = 0; head < queue.size(); ++head) {
            const int i = queue[head];
            if (known[i])
                continue;
            const int x = i % kColumns;
            const int y = i / kColumns;
            float sum = 0.0f;
            int count = 0;
            if (x > 0 && known[i - 1]) { sum += filled[i - 1]; ++count; }
            if (x + 1 < kColumns && known[i + 1]) { sum += filled[i + 1]; ++count; }
            if (y > 0 && known[i - kColumns]) { sum += filled[i - kColumns]; ++count; }
            if (y + 1 < kRows && known[i + kColumns]) { sum += filled[i + kColumns]; ++count; }
            if (count == 0)
                continue;
            filled[i] = sum / float(count);
            known[i] = 1;
            if (x > 0) pushIfUnknown(i - 1);
            if (x + 1 < kColumns) pushIfUnknown(i + 1);
            if (y > 0) pushIfUnknown(i - kColumns);
            if (y + 1 < kRows) pushIfUnknown(i + kColumns);
        }
        for (int i = 0; i < kColumns * kRows; ++i)
            if (!known[i])
                filled[i] = fallback;
    } else {
        for (int y = 0; y < kRows; ++y) {
            for (int x = 0; x < kColumns; ++x) {
                const float u = float(x) / float(kColumns - 1);
                const float v = float(y) / float(kRows - 1);
                filled[y * kColumns + x] = depth.at(u, v);
            }
        }
    }

    // 0.5 is the map's pivot plane, the same value the widget ride uses.
    const float pivotDistance = float(kFarDistance - double(kPivotDepth) * kDepthSpan);
    const auto distanceAtDepth = [&](float value) {
        if (value >= kPivotDepth)
            return pivotDistance - (value - kPivotDepth) * float(kDepthSpan * kNearScale);
        return pivotDistance + (kPivotDepth - value) * float(kDepthSpan * kFarScale);
    };
    const auto sampleDepth = [&](float u, float v) {
        const float value = depth.at(u, v);
        if (!hasMatte)
            return value;
        const float weight = std::clamp(maskAt(hole, u, v), 0.0f, 1.0f);
        return value * (1.0f - weight) + maskAt(filled, u, v) * weight;
    };

    const float aspect = m_outputAspect;
    const float cropU = std::min(1.0f, aspect / m_sourceAspect);
    const float cropV = std::min(1.0f, m_sourceAspect / aspect);
    const float offsetU = (1.0f - cropU) * 0.5f;
    const float offsetV = (1.0f - cropV) * 0.5f;
    const double tangent = std::tan(kFovDegrees * std::numbers::pi / 360.0);

    float minX = std::numeric_limits<float>::max();
    float minY = minX;
    float minZ = minX;
    float maxX = std::numeric_limits<float>::lowest();
    float maxY = maxX;
    float maxZ = maxX;
    double maximumDistance = 0.0;

    const auto makeVertex = [&](float u, float v, float value) {
        const float distance = distanceAtDepth(value);
        maximumDistance = std::max(maximumDistance, double(distance));
        const Vertex vertex{
            float((u * 2.0 - 1.0) * m_imageZoom * distance * tangent * aspect),
            float((1.0 - v * 2.0) * m_imageZoom * distance * tangent),
            -distance,
            float(offsetU + u * cropU),
            float(1.0f - offsetV - v * cropV)
        };
        minX = std::min(minX, vertex.x);
        minY = std::min(minY, vertex.y);
        minZ = std::min(minZ, vertex.z);
        maxX = std::max(maxX, vertex.x);
        maxY = std::max(maxY, vertex.y);
        maxZ = std::max(maxZ, vertex.z);
        return vertex;
    };

    std::array<Vertex, kColumns * kRows> vertices{};
    for (int y = 0; y < kRows; ++y) {
        for (int x = 0; x < kColumns; ++x) {
            const float u = float(x) / float(kColumns - 1);
            const float v = float(y) / float(kRows - 1);
            vertices[y * kColumns + x] = makeVertex(u, v, sampleDepth(u, v));
        }
    }

    std::vector<Vertex> triangles;
    triangles.reserve(size_t(kColumns - 1) * size_t(kRows - 1) * 6);
    const auto append = [&](const Vertex &a, const Vertex &b, const Vertex &c) {
        triangles.push_back(a);
        triangles.push_back(b);
        triangles.push_back(c);
    };

    for (int y = 0; y + 1 < kRows; ++y) {
        for (int x = 0; x + 1 < kColumns; ++x) {
            std::array<std::array<float, kRefine + 1>, kRefine + 1> cellDepth{};
            float cellMin = 1.0f;
            float cellMax = 0.0f;
            for (int sy = 0; sy <= kRefine; ++sy) {
                for (int sx = 0; sx <= kRefine; ++sx) {
                    const float u = float(x * kRefine + sx) / float(kFineColumns - 1);
                    const float v = float(y * kRefine + sy) / float(kFineRows - 1);
                    const float value = sampleDepth(u, v);
                    cellDepth[sy][sx] = value;
                    cellMin = std::min(cellMin, value);
                    cellMax = std::max(cellMax, value);
                }
            }

            const int tl = y * kColumns + x;
            const int tr = tl + 1;
            const int bl = tl + kColumns;
            const int br = bl + 1;
            if (cellMax - cellMin <= kRefineThreshold) {
                append(vertices[tl], vertices[bl], vertices[tr]);
                append(vertices[tr], vertices[bl], vertices[br]);
                continue;
            }

            std::array<Vertex, (kRefine + 1) * (kRefine + 1)> patch;
            for (int sy = 0; sy <= kRefine; ++sy) {
                for (int sx = 0; sx <= kRefine; ++sx) {
                    const float u = float(x * kRefine + sx) / float(kFineColumns - 1);
                    const float v = float(y * kRefine + sy) / float(kFineRows - 1);
                    patch[sy * (kRefine + 1) + sx] = makeVertex(u, v, cellDepth[sy][sx]);
                }
            }
            for (int sy = 0; sy < kRefine; ++sy) {
                for (int sx = 0; sx < kRefine; ++sx) {
                    const int a = sy * (kRefine + 1) + sx;
                    const int b = a + 1;
                    const int c = a + kRefine + 1;
                    const int d = c + 1;
                    append(patch[a], patch[c], patch[b]);
                    append(patch[b], patch[c], patch[d]);
                }
            }
        }
    }

    if (triangles.empty())
        return;

    setVertexData(QByteArray(reinterpret_cast<const char *>(triangles.data()),
        qsizetype(triangles.size() * sizeof(Vertex))));
    setBounds(QVector3D(minX, minY, minZ), QVector3D(maxX, maxY, maxZ));

    const float newBackgroundDistance = float(maximumDistance + 0.25);
    if (std::abs(m_backgroundDistance - newBackgroundDistance) > 1e-5f) {
        m_backgroundDistance = newBackgroundDistance;
        emit backgroundDistanceChanged();
    }
    if (std::abs(m_focusDistance - pivotDistance) > 1e-5f) {
        m_focusDistance = pivotDistance;
        emit focusDistanceChanged();
    }

    setValid(true);
    update();
}
