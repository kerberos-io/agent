/* eslint-env jest */
import { vi } from 'vitest';
import { ImageCanvas } from './ImageCanvas';

vi.mock('vott-ct', () => ({
  CanvasTools: {
    Editor: vi.fn(),
    Core: {
      RegionData: vi.fn(),
      Point2D: vi.fn(),
      Tag: vi.fn(),
      TagsDescriptor: vi.fn(),
      Colors: {
        Color: vi.fn(),
      },
    },
  },
}));

vi.mock('vott-ct/lib/js/CanvasTools/Interface/ISelectorSettings', () => ({
  SelectionMode: {
    NONE: 'none',
    RECT: 'rect',
    POLYGON: 'polygon',
  },
}));

vi.mock('vott-ct/lib/js/CanvasTools/Core/RegionData', () => ({
  RegionDataType: {
    Polygon: 'polygon',
  },
}));

describe('ImageCanvas update guards', () => {
  const callbacks = {
    onAddRegion: vi.fn(),
    onUpdateRegion: vi.fn(),
    onDeleteRegion: vi.fn(),
  };

  const baseProps = () => ({
    image: 'image-a',
    polygons: [],
    device: 'device-1',
    ...callbacks,
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('ignores unrelated parent updates', () => {
    const props = {
      ...baseProps(),
      unrelated: 'next',
    };
    const canvas = new ImageCanvas(props);
    canvas.props = props;
    canvas.loadImage = vi.fn();
    canvas.loadData = vi.fn();

    canvas.componentDidUpdate({
      ...props,
      unrelated: 'previous',
    });

    expect(canvas.loadImage).not.toHaveBeenCalled();
    expect(canvas.loadData).not.toHaveBeenCalled();
  });

  it('reloads the image only when the image source changes', () => {
    const props = baseProps();
    const canvas = new ImageCanvas(props);
    canvas.props = props;
    canvas.loadImage = vi.fn();
    canvas.loadData = vi.fn();

    canvas.componentDidUpdate({
      ...props,
      image: 'image-b',
    });

    expect(canvas.loadImage).toHaveBeenCalledTimes(1);
    expect(canvas.loadImage).toHaveBeenCalledWith(
      'image-a',
      expect.any(Function)
    );
  });

  it('re-renders editor data when polygon props change', () => {
    const props = {
      ...baseProps(),
      polygons: [{ id: '1', coordinates: [] }],
    };
    const canvas = new ImageCanvas(props);
    canvas.props = props;
    canvas.currentImage = {
      width: 100,
      height: 100,
    };
    canvas.loadImage = vi.fn();
    canvas.loadData = vi.fn();

    canvas.componentDidUpdate({
      ...props,
      polygons: [],
    });

    expect(canvas.loadImage).not.toHaveBeenCalled();
    expect(canvas.loadData).toHaveBeenCalledWith(canvas.currentImage);
  });
});
