/* eslint-env jest */
import { ImageCanvas } from './ImageCanvas';

jest.mock('vott-ct', () => ({
  CanvasTools: {
    Editor: jest.fn(),
    Core: {
      RegionData: jest.fn(),
      Point2D: jest.fn(),
      Tag: jest.fn(),
      TagsDescriptor: jest.fn(),
      Colors: {
        Color: jest.fn(),
      },
    },
  },
}));

jest.mock('vott-ct/lib/js/CanvasTools/Interface/ISelectorSettings', () => ({
  SelectionMode: {
    NONE: 'none',
    RECT: 'rect',
    POLYGON: 'polygon',
  },
}));

jest.mock('vott-ct/lib/js/CanvasTools/Core/RegionData', () => ({
  RegionDataType: {
    Polygon: 'polygon',
  },
}));

describe('ImageCanvas update guards', () => {
  const callbacks = {
    onAddRegion: jest.fn(),
    onUpdateRegion: jest.fn(),
    onDeleteRegion: jest.fn(),
  };

  const baseProps = () => ({
    image: 'image-a',
    polygons: [],
    device: 'device-1',
    ...callbacks,
  });

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('ignores unrelated parent updates', () => {
    const props = {
      ...baseProps(),
      unrelated: 'next',
    };
    const canvas = new ImageCanvas(props);
    canvas.props = props;
    canvas.loadImage = jest.fn();
    canvas.loadData = jest.fn();

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
    canvas.loadImage = jest.fn();
    canvas.loadData = jest.fn();

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
    canvas.loadImage = jest.fn();
    canvas.loadData = jest.fn();

    canvas.componentDidUpdate({
      ...props,
      polygons: [],
    });

    expect(canvas.loadImage).not.toHaveBeenCalled();
    expect(canvas.loadData).toHaveBeenCalledWith(canvas.currentImage);
  });
});
