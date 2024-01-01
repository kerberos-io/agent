import React from 'react';
import PropTypes from 'prop-types';
import { CanvasTools } from 'vott-ct';
import { SelectionMode } from 'vott-ct/lib/js/CanvasTools/Interface/ISelectorSettings';
import { RegionDataType } from 'vott-ct/lib/js/CanvasTools/Core/RegionData';
import './ImageCanvas.css';

class ImageCanvas extends React.Component {
  componentDidMount() {
    this.width = 0;
    this.height = 0;

    this.loadImage = this.loadImage.bind(this);
    this.generateRandomTagsDescriptor =
      this.generateRandomTagsDescriptor.bind(this);
    this.tranformPolygon = this.tranformPolygon.bind(this);

    this.editorContainer = document.getElementById('editorDiv');
    this.toolbarContainer = document.getElementById('toolbarDiv');
    this.editor = new CanvasTools.Editor(this.editorContainer);
    const toolset = [
      {
        type: 0,
        action: 'none-select',
        iconFile: 'none-selection.svg',
        tooltip: 'Regions Manipulation (M)',
        key: ['M'],
        actionCallback: (action, rm, sl) => {
          sl.setSelectionMode({ mode: SelectionMode.NONE });
        },
        activate: false,
      },
      {
        type: 0,
        action: 'rect-select',
        iconFile: 'rect-selection.svg',
        tooltip: 'Rectangular box (R)',
        key: ['R'],
        actionCallback: (action, rm, sl) => {
          sl.setSelectionMode({ mode: SelectionMode.RECT });
        },
        activate: true,
      },
      {
        type: 0,
        action: 'polygon-select',
        iconFile: 'polygon-selection.svg',
        key: ['O', ' '],
        tooltip: 'Polygon-selection (O)',
        actionCallback: (action, rm, sl) => {
          sl.setSelectionMode({ mode: SelectionMode.POLYGON });
        },
        activate: false,
      },
    ];

    this.editor.addToolbar(this.toolbarContainer, toolset, '/assets/icons/');

    const { image } = this.props;
    this.loadImage(image, (img) => {
      if (this.width !== img.width || this.height !== img.height) {
        this.width = img.width;
        this.height = img.height;
        this.loadData(img);
      } else {
        this.editor.addContentSource(img);
      }
    });
  }

  componentDidUpdate() {
    const { image } = this.props;
    this.loadImage(image, (img) => {
      if (this.width !== img.width || this.height !== img.height) {
        this.width = img.width;
        this.height = img.height;
        this.loadData(img);
      } else {
        // alert('ok');
        this.editor.addContentSource(img);
      }
    });
  }

  loadData = (image) => {
    const w = image.width;
    const h = image.height;

    this.editor.addContentSource(image).then(() => {
      // Add exisiting polygons
      this.editor.RM.deleteAllRegions();
      const { polygons } = this.props;
      if (polygons) {
        for (let i = 0; i < polygons.length; i += 1) {
          const polygon = polygons[i];
          const regionData = this.tranformPolygon(polygon.coordinates);
          if (regionData) {
            const r = this.editor.scaleRegionToFrameSize(regionData, w, h);
            if (this.editor && this.editor.RM) {
              this.editor.RM.addRegion(
                polygon.id,
                r,
                this.generateRandomTagsDescriptor(polygon.id)
              );
            }
          }
        }
      }

      // Add new region to the editor when new region is created
      this.editor.onSelectionEnd = (regionData) => {
        if (regionData && regionData.x && regionData.y) {
          const id = new Date().getTime().toString();
          const tags = this.generateRandomTagsDescriptor(id);
          const type = RegionDataType.Polygon;
          const r = new CanvasTools.Core.RegionData(
            regionData.x,
            regionData.y,
            regionData.width,
            regionData.height,
            regionData.points,
            type
          );
          this.editor.RM.addRegion(id, r, tags);
          const normalized = this.editor.scaleRegionToSourceSize(r, w, h);
          const { device, onAddRegion } = this.props;
          onAddRegion(device, id, normalized);
        }
      };

      this.editor.onRegionMoveEnd = (id, regionData) => {
        const normalized = this.editor.scaleRegionToSourceSize(
          regionData,
          w,
          h
        );
        const { device, onUpdateRegion } = this.props;
        onUpdateRegion(device, id, normalized);
      };

      this.editor.onRegionDelete = (id, regionData) => {
        const normalized = this.editor.scaleRegionToSourceSize(
          regionData,
          w,
          h
        );
        const { device, onDeleteRegion } = this.props;
        onDeleteRegion(device, id, normalized);
      };
    });
  };

  // eslint-disable-next-line class-methods-use-this
  loadImage = (path, onready) => {
    const image = new Image();
    image.src = path;
    image.addEventListener('load', (e) => {
      onready(e.target);
    });
  };

  // eslint-disable-next-line class-methods-use-this
  generateRandomTagsDescriptor = (id) => {
    const { Color } = CanvasTools.Core.Colors;
    const primaryTags = [new CanvasTools.Core.Tag(id, new Color('#943734'))];
    const primaryTag = primaryTags[0];
    const tags = new CanvasTools.Core.TagsDescriptor([primaryTag]);
    return tags;
  };

  // eslint-disable-next-line class-methods-use-this
  tranformPolygon = (points) => {
    if (!points || points.length === 0) return null;

    let x = -1;
    let y = -1;
    let width = -1;
    let height = -1;

    let minX = points[0].x;
    let maxX = points[0].x;
    let minY = points[0].y;
    let maxY = points[0].y;

    const p = [];
    for (let i = 0; i < points.length; i += 1) {
      // Add to array of points
      p.push(new CanvasTools.Core.Point2D(points[i].x, points[i].y));

      // Check the bounding box.
      if (points[i].x < minX) {
        minX = points[i].x;
      }
      if (points[i].x > maxX) {
        maxX = points[i].x;
      }
      if (points[i].y < minY) {
        minY = points[i].y;
      }
      if (points[i].y > maxY) {
        maxY = points[i].y;
      }
    }

    x = minX;
    y = minY;
    width = maxX - minX;
    height = maxY - minY;

    const region = new CanvasTools.Core.RegionData(
      x,
      y,
      width,
      height,
      p,
      RegionDataType.Polygon
    );
    return region;
  };

  render() {
    return (
      <div id="canvasToolsDiv">
        <div id="toolbarDiv" />
        <div id="selectionDiv">
          <div id="editorDiv" />
        </div>
      </div>
    );
  }
}

ImageCanvas.propTypes = {
  image: PropTypes.string.isRequired,
  polygons: PropTypes.string.isRequired,
  device: PropTypes.string.isRequired,
  onAddRegion: PropTypes.func.isRequired,
  onUpdateRegion: PropTypes.func.isRequired,
  onDeleteRegion: PropTypes.func.isRequired,
};

export default ImageCanvas;
