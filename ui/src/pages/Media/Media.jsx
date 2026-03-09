import React from 'react';
import PropTypes from 'prop-types';
import { withTranslation } from 'react-i18next';
import {
  Breadcrumb,
  ControlBar,
  VideoCard,
  Button,
  Modal,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from '@kerberos-io/ui';
import { Link, withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import { getEvents } from '../../actions/agent';
import config from '../../config';
import './Media.scss';

function formatDateTimeLocal(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');

  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

function getDefaultTimeWindow() {
  const endDate = new Date();
  const startDate = new Date(endDate.getTime() - 60 * 60 * 1000);

  return {
    startDateTime: formatDateTimeLocal(startDate),
    endDateTime: formatDateTimeLocal(endDate),
    timestamp_offset_start: Math.floor(startDate.getTime() / 1000),
    timestamp_offset_end: Math.floor(endDate.getTime() / 1000) + 59,
  };
}

function normalizeInputValue(valueOrEvent) {
  if (valueOrEvent && valueOrEvent.target) {
    return valueOrEvent.target.value;
  }

  return valueOrEvent;
}

// eslint-disable-next-line react/prefer-stateless-function
class Media extends React.Component {
  constructor() {
    super();

    const defaultTimeWindow = getDefaultTimeWindow();

    const initialFilter = {
      timestamp_offset_start: defaultTimeWindow.timestamp_offset_start,
      timestamp_offset_end: defaultTimeWindow.timestamp_offset_end,
      number_of_elements: 12,
    };

    this.state = {
      appliedFilter: initialFilter,
      startDateTime: defaultTimeWindow.startDateTime,
      endDateTime: defaultTimeWindow.endDateTime,
      isScrolling: false,
      open: false,
      currentRecording: '',
    };
  }

  componentDidMount() {
    const { dispatchGetEvents } = this.props;
    const { appliedFilter } = this.state;
    dispatchGetEvents(appliedFilter);
    document.addEventListener('scroll', this.trackScrolling);
  }

  componentWillUnmount() {
    document.removeEventListener('scroll', this.trackScrolling);
  }

  handleClose() {
    this.setState({
      open: false,
      currentRecording: '',
    });
  }

  trackScrolling = () => {
    const { events, dispatchGetEvents } = this.props;
    const { isScrolling, appliedFilter } = this.state;
    const wrappedElement = document.getElementById('loader');
    if (!wrappedElement || isScrolling || !this.isBottom(wrappedElement)) {
      return;
    }

    this.setState({
      isScrolling: true,
    });

    // Get last element
    const lastElement = events[events.length - 1];
    if (lastElement) {
      dispatchGetEvents(
        {
          ...appliedFilter,
          timestamp_offset_end: parseInt(lastElement.timestamp, 10),
        },
        () => {
          setTimeout(() => {
            this.setState({
              isScrolling: false,
            });
          }, 1000);
        },
        () => {
          this.setState({
            isScrolling: false,
          });
        },
        true
      );
    } else {
      this.setState({
        isScrolling: false,
      });
    }
  };

  buildEventFilter(startDateTime, endDateTime) {
    const { appliedFilter } = this.state;

    return {
      timestamp_offset_start: this.getTimestampFromInput(startDateTime, 'start'),
      timestamp_offset_end: this.getTimestampFromInput(endDateTime, 'end'),
      number_of_elements: appliedFilter.number_of_elements,
    };
  }

  handleDateFilterChange(field, value) {
    const { dispatchGetEvents } = this.props;
    const { startDateTime, endDateTime } = this.state;
    const normalizedValue = normalizeInputValue(value);
    const nextStartDateTime =
      field === 'startDateTime' ? normalizedValue : startDateTime;
    const nextEndDateTime = field === 'endDateTime' ? normalizedValue : endDateTime;
    const nextFilter = this.buildEventFilter(nextStartDateTime, nextEndDateTime);
    const shouldApplyFilter =
      (nextStartDateTime === '' || nextStartDateTime.length === 16) &&
      (nextEndDateTime === '' || nextEndDateTime.length === 16);

    this.setState(
      {
        [field]: normalizedValue,
        appliedFilter: shouldApplyFilter ? nextFilter : this.state.appliedFilter,
        isScrolling: false,
      },
      () => {
        if (shouldApplyFilter) {
          dispatchGetEvents(nextFilter);
        }
      }
    );
  }

  getTimestampFromInput(value, boundary) {
    if (!value) {
      return 0;
    }

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return 0;
    }

    const seconds = Math.floor(date.getTime() / 1000);
    if (boundary === 'end') {
      return seconds + 59;
    }
    return seconds;
  }

  isBottom(el) {
    return el.getBoundingClientRect().bottom + 50 <= window.innerHeight;
  }

  openModal(file) {
    this.setState({
      open: true,
      currentRecording: file,
    });
  }

  render() {
    const { events, eventsLoaded, t } = this.props;
    const { isScrolling, open, currentRecording, startDateTime, endDateTime } =
      this.state;

    return (
      <div id="media">
        <Breadcrumb
          title={t('recordings.title')}
          level1={t('recordings.heading')}
          level1Link=""
        >
          <Link to="/settings">
            <Button
              label={t('breadcrumb.configure')}
              icon="preferences"
              type="default"
            />
          </Link>
        </Breadcrumb>

        <div className="media-control-bar">
          <ControlBar>
            <div className="media-filters">
              <div className="media-filters__field">
                <label htmlFor="recordings-start-time">Start time</label>
                <input
                  className="media-filters__input"
                  id="recordings-start-time"
                  type="datetime-local"
                  value={startDateTime}
                  onChange={(value) =>
                    this.handleDateFilterChange('startDateTime', value)
                  }
                />
              </div>
              <div className="media-filters__field">
                <label htmlFor="recordings-end-time">End time</label>
                <input
                  className="media-filters__input"
                  id="recordings-end-time"
                  type="datetime-local"
                  value={endDateTime}
                  onChange={(value) =>
                    this.handleDateFilterChange('endDateTime', value)
                  }
                />
              </div>
            </div>
          </ControlBar>
        </div>

        <div className="stats grid-container --four-columns">
          {events.map((event) => (
            <div
              key={event.key}
              onClick={() => this.openModal(`${config.URL}/file/${event.key}`)}
            >
              <VideoCard
                isMediaWall
                videoSrc={`${config.URL}/file/${event.key}`}
                hours={event.time}
                month={event.short_day}
                videoStatus=""
                duration=""
                headerStatus=""
                headerStatusTitle=""
                handleClickHD={() => true}
                handleClickSD={() => true}
              />
            </div>
          ))}
        </div>
        {events.length === 0 && eventsLoaded === 0 && (
          <div className="media-empty-state">
            No recordings found in the selected time range.
          </div>
        )}
        {open && (
          <Modal>
            <ModalHeader
              title="View recording"
              onClose={() => this.handleClose()}
            />
            <ModalBody>
              <video controls autoPlay>
                <source src={currentRecording} type="video/mp4" />
              </video>
            </ModalBody>
            <ModalFooter
              right={
                <>
                  <a
                    href={currentRecording}
                    download="video"
                    target="_blank"
                    rel="noreferrer"
                  >
                    <Button
                      label="Download"
                      icon="download"
                      type="button"
                      buttonType="button"
                    />
                  </a>
                  <Button
                    label="Close"
                    icon="cross-circle"
                    type="button"
                    buttonType="button"
                    onClick={() => this.handleClose()}
                  />
                </>
              }
            />
          </Modal>
        )}

        {!isScrolling && eventsLoaded !== 0 && (
          <div id="loader">
            <div className="lds-ripple">
              <div />
              <div />
            </div>
          </div>
        )}
      </div>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  events: state.agent.events,
  eventsLoaded: state.agent.eventsLoaded,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchGetEvents: (eventFilter, success, error, append) =>
    dispatch(getEvents(eventFilter, success, error, append)),
});

Media.propTypes = {
  t: PropTypes.func.isRequired,
  events: PropTypes.arrayOf(PropTypes.object).isRequired,
  eventsLoaded: PropTypes.number.isRequired,
  dispatchGetEvents: PropTypes.func.isRequired,
};

export default withTranslation()(
  withRouter(connect(mapStateToProps, mapDispatchToProps)(Media))
);
