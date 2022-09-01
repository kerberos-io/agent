import React from 'react';
import PropTypes from 'prop-types';
import {
  Breadcrumb,
  VideoContainer,
  VideoCard,
  ControlBar,
  Button,
  Input,
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

// eslint-disable-next-line react/prefer-stateless-function
class Media extends React.Component {
  constructor() {
    super();
    this.state = {
      timestamp_offset_start: 0,
      timestamp_offset_end: 0,
      number_of_elements: 12,
      isScrolling: false,
      open: false,
      currentRecording: '',
    };
  }

  componentDidMount() {
    const { dispatchGetEvents } = this.props;
    dispatchGetEvents(this.state);
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
    const { isScrolling } = this.state;
    const wrappedElement = document.getElementById('loader');
    if (!isScrolling && this.isBottom(wrappedElement)) {
      this.setState({
        isScrolling: true,
      });
      // Get last element
      const lastElement = events[events.length - 1];
      if (lastElement) {
        this.setState({
          timestamp_offset_end: parseInt(lastElement.timestamp, 10),
        });
        dispatchGetEvents(this.state, () => {
          setTimeout(() => {
            this.setState({
              isScrolling: false,
            });
          }, 1000);
        });
      }
    }
  };

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
    const { events } = this.props;
    const { isScrolling, open, currentRecording } = this.state;
    return (
      <div id="media">
        <Breadcrumb
          title="Recordings"
          level1="All your recordings in a single place"
          level1Link=""
        >
          <Link to="/settings">
            <Button label="Configure" icon="preferences" type="default" />
          </Link>
        </Breadcrumb>

        <ControlBar>
          <Input
            iconleft="search"
            onChange={() => {}}
            placeholder="Search media..."
            layout="controlbar"
            type="text"
          />
        </ControlBar>

        <VideoContainer cols={4} isVideoWall={false}>
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
        </VideoContainer>
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

        {!isScrolling && (
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
});

const mapDispatchToProps = (dispatch) => ({
  dispatchGetEvents: (eventFilter, success, error) =>
    dispatch(getEvents(eventFilter, success, error)),
});

Media.propTypes = {
  events: PropTypes.objectOf(PropTypes.object).isRequired,
  dispatchGetEvents: PropTypes.func.isRequired,
};

export default withRouter(connect(mapStateToProps, mapDispatchToProps)(Media));
